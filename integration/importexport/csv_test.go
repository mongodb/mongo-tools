// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package importexport

import (
	"os"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestRoundTripFieldFile verifies that mongoexport --fieldFile limits exported
// fields, and that mongoimport correctly restores the filtered data.
func (s *ImportExportSuite) TestRoundTripFieldFile() {
	const dbName = "mongoimport_roundtrip_fieldfile_test"

	client := s.newClient()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	s.Require().NoError(err)

	fieldFile, err := os.CreateTemp(s.T().TempDir(), "fields-*.txt")
	s.Require().NoError(err)
	_, err = fieldFile.WriteString("a\nb\n")
	s.Require().NoError(err)
	s.Require().NoError(fieldFile.Close())

	exportTarget, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err)
	s.Require().NoError(exportTarget.Close())

	exportToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			FieldFile:  fieldFile.Name(),
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()

	f, err := os.OpenFile(exportTarget.Name(), os.O_WRONLY, 0o644)
	s.Require().NoError(err)
	_, err = me.Export(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	fields := "a,b,c"
	importToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       exportTarget.Name(),
			Type:       "csv",
			Fields:     &fields,
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)

	dest := db.Collection("dest")
	n, err := dest.CountDocuments(s.Context(), bson.D{{"a", 1}})
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
	s.Assert().EqualValues(0, n, "c=3 should not have been exported (not in fieldFile)")
}

// TestRoundTripFieldsCSV verifies that mongoexport --csv --fields limits which
// fields are exported, and that mongoimport correctly restores the filtered data.
func (s *ImportExportSuite) TestRoundTripFieldsCSV() {
	const dbName = "mongoimport_roundtrip_fieldscsv_test"

	client := s.newClient()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	s.Require().NoError(err)

	s.exportCSVAndImport(dbName, "a", db)
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

	s.exportCSVAndImport(dbName, "a,b,c", db)
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
	s.Assert().NotEqual(fromSource["_id"], fromDest["_id"], "_id should not have been exported")
}

// TestRoundTripNestedFieldsCSV verifies that mongoexport correctly exports
// nested dotted field paths to CSV and that mongoimport restores them.
func (s *ImportExportSuite) TestRoundTripNestedFieldsCSV() {
	const dbName = "mongoimport_roundtrip_nestedcsv_test"

	client := s.newClient()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 2}, {"b", bson.D{{"c", 2}}}},
		bson.D{{"a", 3}, {"b", bson.D{{"c", 3}, {"d", bson.D{{"e", 3}}}}}},
		bson.D{{"a", 4}, {"x", nil}},
	})
	s.Require().NoError(err)

	exportToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			Fields:     "a,b.d.e,x.y",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err)
	_, err = me.Export(tmpFile)
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	importToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			Type:       "csv",
			HeaderLine: true,
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)

	dest := db.Collection("dest")
	for _, tc := range []struct {
		filter bson.D
		count  int64
		msg    string
	}{
		{bson.D{{"b.c", 2}}, 0, "b.c should not have been exported"},
		{bson.D{{"b.c", 3}}, 0, "b.c should not have been exported"},
		{bson.D{{"b.d.e", 3}}, 1, "b.d.e=3 should be present"},
		{bson.D{{"b.d.e", ""}}, 3, "b.d.e should be empty string for 3 docs"},
		{bson.D{{"a", 1}}, 1, "a=1 should be present"},
		{bson.D{{"a", 2}}, 1, "a=2 should be present"},
		{bson.D{{"a", 3}}, 1, "a=3 should be present"},
		{bson.D{{"x.y", ""}}, 4, "x.y should be empty string for all 4 docs"},
	} {
		n, err := dest.CountDocuments(s.Context(), tc.filter)
		s.Require().NoError(err)
		s.Assert().EqualValues(tc.count, n, tc.msg)
	}
}

func (s *ImportExportSuite) exportCSVAndImport(dbName, exportFields string, db *mongo.Database) {
	s.Require().NoError(db.Collection("dest").Drop(s.Context()))

	exportTarget, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err)
	s.Require().NoError(exportTarget.Close())

	exportToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			Fields:     exportFields,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	f, err := os.OpenFile(exportTarget.Name(), os.O_WRONLY, 0o644)
	s.Require().NoError(err)
	_, err = me.Export(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	importFields := "a,b,c"
	importToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       exportTarget.Name(),
			Type:       "csv",
			Fields:     &importFields,
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)
}
