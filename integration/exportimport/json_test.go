// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package exportimport

import (
	"os"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestRoundTripFieldsJSON verifies that mongoexport --fields limits which fields
// appear in JSON export output, and that _id is included (unlike CSV).
func (s *ExportImportSuite) TestRoundTripFieldsJSON() {
	const dbName = "mongoimport_roundtrip_fieldsjson_test"

	client := s.Client()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	s.Require().NoError(err)

	s.exportJSONAndImport(dbName, "a", db)
	dest := db.Collection("dest")
	n, err := dest.CountDocuments(s.Context(), bson.D{{"a", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(3, n, "3 documents should have a=1")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "b=1 should not have been exported")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 2}})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "b=2 should not have been exported")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"c", 3}})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "c=3 should not have been exported")

	s.exportJSONAndImport(dbName, "a,b,c", db)
	n, err = dest.CountDocuments(s.Context(), bson.D{{"a", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(3, n, "3 documents should have a=1")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "1 document should have b=1")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 2}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "1 document should have b=2")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"c", 3}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "1 document should have c=3")

	var fromSource, fromDest bson.M
	q := bson.D{{"a", 1}, {"b", 1}}
	err = db.Collection("source").FindOne(s.Context(), q).Decode(&fromSource)
	s.Require().NoError(err)
	err = dest.FindOne(s.Context(), q).Decode(&fromDest)
	s.Require().NoError(err)
	s.Assert().Equal(
		fromSource["_id"], fromDest["_id"],
		"_id should have been exported in JSON mode",
	)
}

// TestRoundTripJSONArray verifies that mongoexport --jsonArray produces a JSON
// array, that mongoimport rejects it without --jsonArray, and accepts it with
// --jsonArray.
func (s *ExportImportSuite) TestRoundTripJSONArray() {
	const dbName = "mongoimport_roundtrip_jsonarray_test"
	const collName = "data"

	client := s.Client()

	coll := client.Database(dbName).Collection(collName)
	docs := make([]any, 20)
	for i := range 20 {
		docs[i] = bson.D{{"_id", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err)

	exportToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
			JSONArray:  true,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	_, err = me.Export(tmpFile)
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	s.Require().NoError(coll.Drop(s.Context()))

	importWithoutFlagOpts, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	importWithoutFlagOpts.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importWithoutFlagOpts,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name(), ParseGrace: "stop"},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Assert().Error(err, "import without --jsonArray should fail on jsonArray output")

	n, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "nothing should have been imported without --jsonArray")

	importToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err = mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			ParseGrace: "stop",
			JSONArray:  true,
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	imported, _, err := mi.ImportDocuments()
	s.Require().NoError(err)
	s.Assert().EqualValues(20, imported, "should import all 20 documents with --jsonArray")

	n, err = coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err)
	s.Assert().EqualValues(20, n, "all 20 documents should be present after import")
	for i := range 20 {
		c, err := coll.CountDocuments(s.Context(), bson.D{{"_id", i}})
		s.Require().NoError(err)
		s.Assert().EqualValues(1, c, "document with _id %d should exist", i)
	}
}

func (s *ExportImportSuite) exportJSONAndImport(dbName, fields string, db *mongo.Database) {
	s.Require().NoError(db.Collection("dest").Drop(s.Context()))

	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	exportToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
			Fields:     fields,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	f, err := os.OpenFile(tmpFile.Name(), os.O_WRONLY, 0o644)
	s.Require().NoError(err)
	_, err = me.Export(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	importToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)
}
