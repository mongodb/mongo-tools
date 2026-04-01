// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package importexport

import (
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
)

// TestRoundTripBasicData verifies that data exported by mongoexport can be
// fully restored by mongoimport with all documents intact.
func TestRoundTripBasicData(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_basic_test"
	const collName = "data"

	client := newTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	var docs []bson.D
	for i := range 50 {
		docs = append(docs, bson.D{{"_id", int32(i)}})
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}

	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)

	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	require.NoError(t, err)
	defer me.Close()
	_, err = me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	require.NoError(t, coll.Drop(t.Context()))

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importToolOptions,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name()},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, 50, imported, "should import all 50 documents")

	count, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 50, count, "collection should have all 50 documents after round-trip")

	for i := range 50 {
		c, err := coll.CountDocuments(t.Context(), bson.D{{"_id", i}})
		require.NoError(t, err)
		assert.EqualValues(t, 1, c, "document with _id %d should exist after round-trip", i)
	}
}

// TestRoundTripDataTypes verifies that documents with diverse BSON types
// survive an export-then-import round-trip intact.
func TestRoundTripDataTypes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_datatypes_test"
	const collName = "data"

	client := newTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	docs := []any{
		bson.D{{"num", 1}},
		bson.D{{"flt", 1.0}},
		bson.D{{"str", "1"}},
		bson.D{{"obj", bson.D{{"a", 1}}}},
		bson.D{{"arr", bson.A{0, 1}}},
		bson.D{{"bd", bson.Binary{Subtype: 0x00, Data: []byte{0xd7, 0x6d, 0xf8}}}},
		bson.D{
			{
				"date",
				bson.NewDateTimeFromTime(time.Date(2009, 8, 27, 12, 34, 56, 789000000, time.UTC)),
			},
		},
		bson.D{{"ts", bson.Timestamp{T: 1234, I: 5678}}},
		bson.D{{"rx", bson.Regex{Pattern: `foo*"bar"`, Options: "i"}}},
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}

	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)

	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	require.NoError(t, err)
	defer me.Close()
	_, err = me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	require.NoError(t, coll.Drop(t.Context()))

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importToolOptions,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name()},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, 9, imported, "should import all 9 documents")

	count, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 9, count, "collection should have all 9 documents after round-trip")

	for _, q := range []bson.D{
		{{"num", 1}},
		{{"flt", 1.0}},
		{{"str", "1"}},
		{{"obj", bson.D{{"a", 1}}}},
		{{"arr", bson.A{0, 1}}},
		{{"rx", bson.D{{"$exists", true}}}},
	} {
		c, err := coll.CountDocuments(t.Context(), q)
		require.NoError(t, err)
		assert.EqualValues(t, 1, c, "document matching %v should exist after round-trip", q)
	}
}

// TestRoundTripDecimal128 verifies that a Decimal128 value survives an
// export-then-import round-trip.
func TestRoundTripDecimal128(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_decimal128_test"
	const collName = "dec128"

	client := newTestClient(t, dbName)

	dec, err := bson.ParseDecimal128("123456789012345678901234567890")
	require.NoError(t, err)
	testDoc := bson.D{{"_id", "foo"}, {"x", dec}}

	coll := client.Database(dbName).Collection(collName)
	_, err = coll.InsertOne(t.Context(), testDoc)
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
	assert.EqualValues(t, 1, imported, "should import 1 document")

	var result bson.D
	err = coll.FindOne(t.Context(), bson.D{{"_id", "foo"}}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, testDoc, result, "imported doc should match original")
}

// TestRoundTripViewExport verifies that mongoexport correctly exports documents
// from a MongoDB view, and that mongoimport can restore them.
func TestRoundTripViewExport(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_roundtrip_views_test"

	client := newTestClient(t, dbName)

	db := client.Database(dbName)

	cities := []any{
		bson.D{{"city", "Boise"}, {"state", "ID"}},
		bson.D{{"city", "Pocatello"}, {"state", "ID"}},
		bson.D{{"city", "Nampa"}, {"state", "ID"}},
		bson.D{{"city", "Albany"}, {"state", "NY"}},
		bson.D{{"city", "New York"}, {"state", "NY"}},
		bson.D{{"city", "Los Angeles"}, {"state", "CA"}},
		bson.D{{"city", "San Jose"}, {"state", "CA"}},
		bson.D{{"city", "Cupertino"}, {"state", "CA"}},
		bson.D{{"city", "San Francisco"}, {"state", "CA"}},
	}
	_, err := db.Collection("cities").InsertMany(t.Context(), cities)
	require.NoError(t, err)

	for _, view := range []struct{ name, state string }{
		{"citiesID", "ID"},
		{"citiesNY", "NY"},
		{"citiesCA", "CA"},
	} {
		pipeline := bson.A{bson.D{{"$match", bson.D{{"state", view.state}}}}}
		err = db.CreateView(t.Context(), view.name, "cities", pipeline)
		require.NoError(t, err)
	}

	n, err := db.Collection("citiesID").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, n, "should have 3 cities in Idaho view")
	n, err = db.Collection("citiesNY").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "should have 2 cities in New York view")
	n, err = db.Collection("citiesCA").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 4, n, "should have 4 cities in California view")

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "citiesCA"}

	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)

	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	require.NoError(t, err)
	defer me.Close()
	_, err = me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	require.NoError(t, db.Drop(t.Context()))

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "CACities"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importToolOptions,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name()},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, 4, imported, "import should succeed")

	n, err = db.Collection("CACities").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 4, n, "restored view should have correct number of rows")
}
