// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package importexport

import (
	"math/rand"
	"os"
	"testing"
	"time"

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

// TestRoundTripLimit verifies that mongoexport --limit restricts the number of
// exported documents, and that the correct documents are restored.
func TestRoundTripLimit(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_limit_test"
	const collName = "data"

	client := newTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	docs := make([]any, 50)
	for i := range 50 {
		docs[i] = bson.D{{"a", i}}
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
		},
		InputOptions: &mongoexport.InputOptions{Sort: "{a:1}", Limit: 20},
	})
	require.NoError(t, err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	n, err := me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	assert.EqualValues(t, 20, n, "should export exactly 20 documents")

	require.NoError(t, coll.Drop(t.Context()))

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importToolOptions,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name(), ParseGrace: "stop"},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, 20, imported, "should import all 20 exported documents")

	count, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 20, count, "collection should have exactly 20 documents")
	for i := range 20 {
		c, err := coll.CountDocuments(t.Context(), bson.D{{"a", i}})
		require.NoError(t, err)
		assert.EqualValues(t, 1, c, "document with a=%d should exist (first 20 by sort)", i)
	}
}

// TestRoundTripQuery verifies that mongoexport --query and --queryFile filter
// export output correctly across multiple query types.
func TestRoundTripQuery(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_query_test"

	client := newTestClient(t, dbName)

	db := client.Database(dbName)

	basicDocs := []any{
		bson.D{{"a", 1}, {"x", bson.D{{"b", "1"}}}},
		bson.D{{"a", 2}, {"x", bson.D{{"b", "1"}, {"c", "2"}}}},
		bson.D{{"a", 1}, {"c", "1"}},
		bson.D{{"a", 2}, {"c", "2"}},
	}

	n := exportAndImportWithQuery(t, db, basicDocs, `{"a":3}`, "")
	assert.EqualValues(t, 0, n, "query matching nothing should export 0 docs")

	n = exportAndImportWithQuery(t, db, basicDocs, `{"a":1,"c":"1"}`, "")
	assert.EqualValues(t, 1, n, "query matching one doc should export 1 doc")

	queryFile, err := os.CreateTemp(t.TempDir(), "query-*.json")
	require.NoError(t, err)
	_, err = queryFile.WriteString(`{"a":1,"c":"1"}`)
	require.NoError(t, err)
	require.NoError(t, queryFile.Close())
	n = exportAndImportWithQuery(t, db, basicDocs, "", queryFile.Name())
	assert.EqualValues(t, 1, n, "queryFile matching one doc should export 1 doc")

	n = exportAndImportWithQuery(t, db, basicDocs, `{"a":2,"x.c":"2"}`, "")
	assert.EqualValues(t, 1, n, "query on embedded doc field should export 1 doc")

	n = exportAndImportWithQuery(t, db, basicDocs, `{}`, "")
	assert.EqualValues(t, 4, n, "empty query should export all 4 docs")

	// TOOLS-469: extended JSON date query with $numberLong
	dateDocs := []any{bson.D{
		{"a", 1},
		{"x", bson.NewDateTimeFromTime(time.Date(2014, 12, 11, 13, 52, 39, 498000000, time.UTC))},
		{"y", bson.NewDateTimeFromTime(time.Date(2014, 12, 13, 13, 52, 39, 498000000, time.UTC))},
	}}
	dateQueryNumberLong := `{
		"x": {
			"$gt": {"$date": {"$numberLong": "1418305949498"}},
			"$lt": {"$date": {"$numberLong": "1418305979498"}}
		},
		"y": {
			"$gt": {"$date": {"$numberLong": "1418478749498"}},
			"$lt": {"$date": {"$numberLong": "1418478769498"}}
		}
	}`
	n = exportAndImportWithQuery(t, db, dateDocs, dateQueryNumberLong, "")
	assert.EqualValues(t, 1, n, "extended JSON date query should export 1 doc")

	// TOOLS-530: date query with ISO string format
	n = exportAndImportWithQuery(
		t,
		db,
		dateDocs,
		`{"x":{"$gt":{"$date":"2014-12-11T13:52:39.3Z"},"$lt":{"$date":"2014-12-11T13:52:39.5Z"}}}`,
		"",
	)
	assert.EqualValues(t, 1, n, "ISO date string query should export 1 doc")
}

// TestRoundTripSortAndSkip verifies that mongoexport --sort and --skip
// correctly affect which documents are exported.
func TestRoundTripSortAndSkip(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_sortskip_test"
	const collName = "data"

	client := newTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	docs := make([]any, 50)
	for i := range 50 {
		docs[i] = bson.D{{"a", i}}
	}
	rand.Shuffle(len(docs), func(i, j int) {
		docs[i], docs[j] = docs[j], docs[i]
	})

	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type: "json", JSONFormat: "relaxed",
		},
		InputOptions: &mongoexport.InputOptions{Sort: "{a:1}", Skip: 20},
	})
	require.NoError(t, err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	n, err := me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	assert.EqualValues(t, 30, n, "should export 30 documents after skipping 20")

	require.NoError(t, coll.Drop(t.Context()))

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importToolOptions,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name(), ParseGrace: "stop"},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, 30, imported, "should import all 30 exported documents")

	count, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 30, count, "collection should have 30 documents")
	for i := range 30 {
		c, err := coll.CountDocuments(t.Context(), bson.D{{"a", i + 20}})
		require.NoError(t, err)
		assert.EqualValues(t, 1, c, "document with a=%d should exist", i+20)
	}
}

func exportAndImportWithQuery(
	t *testing.T,
	db *mongo.Database,
	sourceDocs []any,
	query, queryFile string,
) int64 {
	t.Helper()
	dbName := db.Name()
	require.NoError(t, db.Collection("source").Drop(t.Context()))
	require.NoError(t, db.Collection("dest").Drop(t.Context()))
	if len(sourceDocs) > 0 {
		_, err := db.Collection("source").InsertMany(t.Context(), sourceDocs)
		require.NoError(t, err)
	}
	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type: "json", JSONFormat: "relaxed",
		},
		InputOptions: &mongoexport.InputOptions{Query: query, QueryFile: queryFile},
	})
	require.NoError(t, err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	_, err = me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importToolOptions,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name(), ParseGrace: "stop"},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	_, _, err = mi.ImportDocuments()
	require.NoError(t, err)

	n, err := db.Collection("dest").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	return n
}
