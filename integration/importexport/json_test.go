// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package importexport

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestRoundTripFieldsJSON verifies that mongoexport --fields limits which fields
// appear in JSON export output, and that _id is included (unlike CSV).
func TestRoundTripFieldsJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_fieldsjson_test"

	client := newTestClient(t, dbName)

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(t.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	require.NoError(t, err)

	exportJSONAndImport(t, dbName, "a", db)
	dest := db.Collection("dest")
	n, err := dest.CountDocuments(t.Context(), bson.D{{"a", 1}})
	require.NoError(t, err)
	assert.EqualValues(t, 3, n, "3 documents should have a=1")
	n, err = dest.CountDocuments(t.Context(), bson.D{{"b", 1}})
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "b=1 should not have been exported")
	n, err = dest.CountDocuments(t.Context(), bson.D{{"b", 2}})
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "b=2 should not have been exported")
	n, err = dest.CountDocuments(t.Context(), bson.D{{"c", 3}})
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "c=3 should not have been exported")

	exportJSONAndImport(t, dbName, "a,b,c", db)
	n, err = dest.CountDocuments(t.Context(), bson.D{{"a", 1}})
	require.NoError(t, err)
	assert.EqualValues(t, 3, n, "3 documents should have a=1")
	n, err = dest.CountDocuments(t.Context(), bson.D{{"b", 1}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "1 document should have b=1")
	n, err = dest.CountDocuments(t.Context(), bson.D{{"b", 2}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "1 document should have b=2")
	n, err = dest.CountDocuments(t.Context(), bson.D{{"c", 3}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "1 document should have c=3")

	var fromSource, fromDest bson.M
	q := bson.D{{"a", 1}, {"b", 1}}
	err = db.Collection("source").FindOne(t.Context(), q).Decode(&fromSource)
	require.NoError(t, err)
	err = dest.FindOne(t.Context(), q).Decode(&fromDest)
	require.NoError(t, err)
	assert.Equal(
		t, fromSource["_id"], fromDest["_id"],
		"_id should have been exported in JSON mode",
	)
}

// TestRoundTripJSONArray verifies that mongoexport --jsonArray produces a JSON
// array, that mongoimport rejects it without --jsonArray, and accepts it with
// --jsonArray.
func TestRoundTripJSONArray(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_jsonarray_test"
	const collName = "data"

	client := newTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	docs := make([]any, 20)
	for i := range 20 {
		docs[i] = bson.D{{"_id", i}}
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
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
	require.NoError(t, err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	_, err = me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	require.NoError(t, coll.Drop(t.Context()))

	importWithoutFlagOpts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importWithoutFlagOpts.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importWithoutFlagOpts,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name(), ParseGrace: "stop"},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	_, _, err = mi.ImportDocuments()
	assert.Error(t, err, "import without --jsonArray should fail on jsonArray output")

	n, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "nothing should have been imported without --jsonArray")

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
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
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, 20, imported, "should import all 20 documents with --jsonArray")

	n, err = coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 20, n, "all 20 documents should be present after import")
	for i := range 20 {
		c, err := coll.CountDocuments(t.Context(), bson.D{{"_id", i}})
		require.NoError(t, err)
		assert.EqualValues(t, 1, c, "document with _id %d should exist", i)
	}
}

func exportJSONAndImport(t *testing.T, dbName, fields string, db *mongo.Database) {
	t.Helper()
	require.NoError(t, db.Collection("dest").Drop(t.Context()))

	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
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
	require.NoError(t, err)
	defer me.Close()
	f, err := os.OpenFile(tmpFile.Name(), os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = me.Export(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	_, _, err = mi.ImportDocuments()
	require.NoError(t, err)
}
