// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	// database with test data.
	testDB             = "mongoexport_test_db"
	testCollectionName = "coll1"
)

func simpleMongoExportOpts() (Options, error) {
	toolOptions, err := testutil.GetToolOptions()
	if err != nil {
		return Options{}, fmt.Errorf(
			"error getting tool options to create a mongoexport instance: %w",
			err,
		)
	}

	// Limit ToolOptions to test database
	toolOptions.Namespace = &options.Namespace{DB: testDB, Collection: testCollectionName}

	opts := Options{
		ToolOptions: toolOptions,
		OutputFormatOptions: &OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &InputOptions{},
	}

	log.SetVerbosity(toolOptions.Verbosity)

	return opts, nil
}

func TestExtendedJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	x := bson.M{
		"_id": bson.NewObjectID(),
		"hey": "sup",
		"subdoc": bson.M{
			"subid": bson.NewObjectID(),
		},
		"array": []any{
			bson.NewObjectID(),
			bson.Undefined{},
		},
	}
	out, err := bsonutil.ConvertBSONValueToLegacyExtJSON(x)
	require.NoError(t, err)

	jsonEncoder := json.NewEncoder(os.Stdout)
	err = jsonEncoder.Encode(out)
	require.NoError(t, err)
}

func TestFieldSelect(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	assert.Equal(t, bson.M{"_id": 1, "a": 1, "b": 1}, makeFieldSelector("a,b"))
	assert.Equal(t, bson.M{"_id": 1}, makeFieldSelector(""))
	assert.Equal(t, bson.M{"_id": 1, "foo": 1, "x": 1}, makeFieldSelector("x,foo.baz"))
}

// Test exporting a collection with autoIndexId:false.  As of MongoDB 4.0,
// this is only allowed on the 'local' database.
func TestMongoExportTOOLS2174(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "no cluster available")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "could not get server version")

	if serverVersion.GTE(db.Version{8, 2, 0}) {
		t.Skipf(
			"createCollection no longer accepts autoIndexID as of Server version 8.2.0; testing with %s",
			serverVersion.String(),
		)
	}

	collName := "tools-2174"
	dbName := "local"

	var r1 bson.M
	err = sessionProvider.Run(bson.D{{"drop", collName}}, &r1, dbName)
	if err != nil {
		var commandErr mongo.CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != 26 {
			t.Fatalf("Failed to run drop: %v", err)
		}
	}

	createCmd := bson.D{
		{"create", collName},
		{"autoIndexId", false},
	}
	var r2 bson.M
	err = sessionProvider.Run(createCmd, &r2, dbName)
	require.NoError(t, err, "error creating capped, no-autoIndexId collection")

	t.Run("dumping a capped, autoIndexId:false collection", func(t *testing.T) {
		opts, err := simpleMongoExportOpts()
		require.NoError(t, err)

		opts.Collection = collName
		opts.DB = dbName

		me, err := New(opts)
		require.NoError(t, err)
		defer me.Close()
		out := &bytes.Buffer{}
		_, err = me.Export(out)
		require.NoError(t, err)
	})
}

// Test exporting a collection, _id should only be hinted iff
// this is not a wired tiger collection.
func TestMongoExportTOOLS1952(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "no cluster available")

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "failed to get session")

	collName := "tools-1952-export"
	dbName := "test"
	ns := dbName + "." + collName

	dbStruct := session.Database(dbName)

	var r1 bson.M
	err = sessionProvider.Run(bson.D{{"drop", collName}}, &r1, dbName)
	if err != nil {
		var commandErr mongo.CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != 26 {
			t.Fatalf("Failed to run drop: %v", err)
		}
	}

	createCmd := bson.D{
		{"create", collName},
	}

	var r2 bson.M
	err = sessionProvider.Run(createCmd, &r2, dbName)
	require.NoError(t, err, "error creating collection")

	// Turn on profiling.
	profileCmd := bson.D{
		{"profile", 2},
	}

	err = sessionProvider.Run(profileCmd, &r2, dbName)
	require.NoError(t, err, "failed to turn on profiling")

	profileCollection := dbStruct.Collection("system.profile")

	t.Run("export collection", func(t *testing.T) {
		opts, err := simpleMongoExportOpts()
		require.NoError(t, err)

		opts.Collection = collName
		opts.DB = dbName

		me, err := New(opts)
		require.NoError(t, err)
		defer me.Close()
		out := &bytes.Buffer{}
		_, err = me.Export(out)
		require.NoError(t, err)

		count, err := profileCollection.CountDocuments(t.Context(),
			bson.D{
				{"ns", ns},
				{"op", "query"},
				{"$or", []any{
					// 4.0+
					bson.D{{"command.hint._id", 1}},
					// 3.6
					bson.D{{"command.$nsapshot", true}},
					bson.D{{"command.snapshot", true}},
					// 3.4 and previous
					bson.D{{"query.$snapshot", true}},
					bson.D{{"query.snapshot", true}},
					bson.D{{"query.hint._id", 1}},
				}},
			},
		)
		require.NoError(t, err)

		// In modern storage engines, there should be no hints, so there
		// should be 0 matches.
		assert.Zero(t, count)
	})
}

func TestBadOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	log.SetWriter(io.Discard)

	dbName := "test"
	collName := "mongoexport-bad-options"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}

	type optionsTestCase struct {
		name          string
		optionsFunc   func(Options) Options
		errorTestFunc func(*testing.T, error)
	}

	testCases := []optionsTestCase{
		{
			name: "missing collection",
			optionsFunc: func(o Options) Options {
				o.Collection = ""
				return o
			},
			errorTestFunc: func(t *testing.T, err error) {
				require.Contains(t, err.Error(), "must specify a collection")
			},
		},
		{
			name: "bad JSON in query",
			optionsFunc: func(o Options) Options {
				o.Query = "{ hello }"
				return o
			},
			errorTestFunc: func(t *testing.T, err error) {
				require.Regexp(t, `query.+is not valid JSON`, err.Error())
			},
		},
		{
			name: "invalid sort",
			optionsFunc: func(o Options) Options {
				o.Sort = "{ hello }"
				return o
			},
			errorTestFunc: func(t *testing.T, err error) {
				require.Regexp(t, `query.+is not valid JSON`, err.Error())
			},
		},
		{
			name: "query file does not exist",
			optionsFunc: func(o Options) Options {
				o.QueryFile = "does/not/exist.json"
				return o
			},
			errorTestFunc: func(t *testing.T, err error) {
				if runtime.GOOS == "windows" {
					require.Contains(t, err.Error(), "cannot find the path specified")
				} else {
					require.Contains(t, err.Error(), "no such file or directory")
				}
			},
		},
	}

	for _, testCase := range testCases {
		require.NoError(t, sessionProvider.DropCollection(dbName, collName))

		require.NoError(t, sessionProvider.CreateCollection(dbName, collName))

		opts, err := simpleMongoExportOpts()
		require.NoError(t, err)

		opts = testCase.optionsFunc(opts)

		_, err = New(opts)
		require.Error(t, err)
		if testCase.errorTestFunc != nil {
			testCase.errorTestFunc(t, err)
		}
	}
}

func TestMongoExportTimeseries(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "no cluster available")

	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "no client available")

	fcv := testutil.GetFCV(client)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 5.0 or later; found %v", fcv)
	}

	dbName := "test_ts"
	collName := "tscoll"

	db := client.Database(dbName)
	err = db.Drop(t.Context())
	require.NoError(t, err)

	setUpTimeseries(t, dbName, collName)

	t.Run("export logical documents", func(t *testing.T) {
		opts, err := simpleMongoExportOpts()
		require.NoError(t, err)

		opts.Collection = collName
		opts.DB = dbName

		me, err := New(opts)
		require.NoError(t, err)
		defer me.Close()
		out := new(bytes.Buffer)
		count, err := me.Export(out)
		require.NoError(t, err)
		assert.EqualValues(t, 1000, count)
	})

	t.Run("export bucket documents", func(t *testing.T) {
		opts, err := simpleMongoExportOpts()
		require.NoError(t, err)

		opts.Collection = "system.buckets." + collName
		opts.DB = dbName

		me, err := New(opts)
		require.NoError(t, err)
		defer me.Close()

		out := new(bytes.Buffer)
		count, err := me.Export(out)

		require.NoError(t, err)
		assert.EqualValues(t, 10, count)
	})
}

func setUpTimeseries(t *testing.T, dbName string, collName string) {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	timeseriesOptions := bson.D{
		{"timeField", "ts"},
		{"metaField", "my_meta"},
	}
	createCmd := bson.D{
		{"create", collName},
		{"timeseries", timeseriesOptions},
	}
	var r2 bson.D
	err = sessionProvider.Run(createCmd, &r2, dbName)
	require.NoError(t, err, "create timeseries coll")

	coll := sessionProvider.DB(dbName).Collection(collName)

	idx := mongo.IndexModel{
		Keys: bson.D{{"my_meta.device", 1}},
	}
	_, err = coll.Indexes().CreateOne(t.Context(), idx)
	require.NoError(t, err, "create index 1")

	idx = mongo.IndexModel{
		Keys: bson.D{{"ts", 1}, {"my_meta.device", 1}},
	}
	_, err = coll.Indexes().CreateOne(t.Context(), idx)
	require.NoError(t, err, "create index 2")

	for i := range 1000 {
		metadata := bson.M{
			"device": i % 10,
		}
		_, err = coll.InsertOne(
			t.Context(),
			bson.M{
				"ts":          bson.NewDateTimeFromTime(time.Now()),
				"my_meta":     metadata,
				"measurement": i,
			},
		)

		require.NoError(t, err, "insert ts data (%d)", i)
	}
}
