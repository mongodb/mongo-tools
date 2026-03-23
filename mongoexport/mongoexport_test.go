// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

// TestExportNestedFieldsCSV verifies that mongoexport correctly handles nested
// field paths and $ projection in --fields with --csv output.
func TestExportNestedFieldsCSV(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	const dbName = "mongoexport_nestedfieldscsv_test"
	const collName = "source"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)
	client, err := sessionProvider.GetSession()
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := client.Database(dbName).Drop(context.Background()); err != nil {
			t.Errorf("dropping test database: %v", err)
		}
	})

	coll := client.Database(dbName).Collection(collName)
	_, err = coll.InsertMany(t.Context(), []any{
		bson.D{{"a", bson.A{1, 2, 3, 4, 5}}, {"b", bson.D{{"c", bson.A{-1, -2, -3, -4}}}}},
		bson.D{
			{"a", int32(1)},
			{"b", int32(2)},
			{"c", int32(3)},
			{"d", bson.D{{"e", bson.A{4, 5, 6}}}},
		},
		bson.D{
			{"a", int32(1)},
			{"b", int32(2)},
			{"c", int32(3)},
			{"d", int32(5)},
			{"e", bson.D{{"0", bson.A{"foo", "bar", "baz"}}}},
		},
		bson.D{
			{"a", int32(1)},
			{"b", int32(2)},
			{"c", int32(3)},
			{"d", bson.A{4, 5, 6}},
			{"e", bson.A{bson.D{{"0", 0}, {"1", 1}}, bson.D{{"2", 2}, {"3", 3}}}},
		},
	})
	require.NoError(t, err)

	toolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}

	rows := parseCSVRows(t, exportNestedCSV(t, toolOptions, "d.e.2", ""))
	assert.True(
		t, rowsContainValue(rows, "d.e.2", "6"),
		"d.e.2 should select the third element of d.e array",
	)

	rows = parseCSVRows(t, exportNestedCSV(t, toolOptions, "e.0.0", ""))
	assert.True(
		t, rowsContainValue(rows, "e.0.0", "foo"),
		"e.0.0 should select nested numeric array value",
	)

	rows = parseCSVRows(t, exportNestedCSV(t, toolOptions, "b,d.1,e.1.3", ""))
	assert.True(t, rowsContainValue(rows, "b", "2"), "b column should contain 2")
	assert.True(t, rowsContainValue(rows, "d.1", "5"), "d.1 column should contain 5")
	assert.True(t, rowsContainValue(rows, "e.1.3", "3"), "e.1.3 column should contain 3")

	// $ projection strips the trailing .$ from the header name and wraps the result in [].
	rows = parseCSVRows(t, exportNestedCSV(t, toolOptions, "d.$", `{"d": 4}`))
	assert.True(
		t, rowsContainValue(rows, "d", "[4]"),
		"d.$ with query {d:4} should select matching element",
	)

	rows = parseCSVRows(t, exportNestedCSV(t, toolOptions, "a.$", `{"a": {"$gt": 1}}`))
	assert.True(
		t, rowsContainValue(rows, "a", "[2]"),
		"a.$ with query {a:{$gt:1}} should select matching element",
	)

	rows = parseCSVRows(t, exportNestedCSV(t, toolOptions, "b.c.$", `{"b.c": -1}`))
	assert.True(
		t, rowsContainValue(rows, "b.c", "[-1]"),
		"b.c.$ with query {b.c:-1} should select matching element",
	)
}

func exportNestedCSV(t *testing.T, toolOptions *options.ToolOptions, fields, query string) string {
	t.Helper()
	me, err := New(Options{
		ToolOptions: toolOptions,
		OutputFormatOptions: &OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			Fields:     fields,
		},
		InputOptions: &InputOptions{Query: query},
	})
	require.NoError(t, err)
	defer me.Close()
	var buf bytes.Buffer
	_, err = me.Export(&buf)
	require.NoError(t, err)
	return buf.String()
}

func parseCSVRows(t *testing.T, output string) []map[string]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(output))
	records, err := r.ReadAll()
	require.NoError(t, err)
	if len(records) == 0 {
		return nil
	}
	headers := records[0]
	rows := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(record) {
				row[h] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func rowsContainValue(rows []map[string]string, col, val string) bool {
	for _, row := range rows {
		if row[col] == val {
			return true
		}
	}
	return false
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

// TestBrokenPipe verifies that mongoexport handles a broken pipe gracefully
// (exits with a write error rather than being killed by SIGPIPE).
func TestBrokenPipe(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongoexport_broken_pipe_test"
		collName = "docs"
	)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)
	client, err := sessionProvider.GetSession()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Database(dbName).Drop(context.Background())
	})

	// Insert 1000 docs with a large field so JSON output exceeds the pipe buffer.
	docs := make([]any, 1000)
	for i := range 1000 {
		docs[i] = bson.D{{"_id", int32(i)}, {"data", strings.Repeat("x", 1000)}}
	}
	_, err = client.Database(dbName).Collection(collName).InsertMany(context.Background(), docs)
	require.NoError(t, err)

	args := append(
		[]string{"run", filepath.Join("..", "mongoexport", "main")},
		testutil.GetBareArgs()...,
	)
	args = append(args, "--db", dbName, "--collection", collName)
	testutil.AssertBrokenPipeHandled(t, exec.Command("go", args...))
}

// TestExportNamespaceValidation verifies that mongoexport rejects invalid DB
// names and accepts system collections.
func TestExportNamespaceValidation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	for _, dbName := range []string{"test.bar", `test"bar`} {
		toolOptions, err := testutil.GetToolOptions()
		require.NoError(t, err)
		toolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "foo"}
		_, err = New(Options{
			ToolOptions:         toolOptions,
			OutputFormatOptions: &OutputFormatOptions{Type: "json", JSONFormat: "canonical"},
			InputOptions:        &InputOptions{},
		})
		assert.Error(t, err, "db name %q should be rejected", dbName)
	}

	toolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOptions.Namespace = &options.Namespace{DB: "test", Collection: "system.foobar"}
	me, err := New(Options{
		ToolOptions:         toolOptions,
		OutputFormatOptions: &OutputFormatOptions{Type: "json", JSONFormat: "canonical"},
		InputOptions:        &InputOptions{},
	})
	assert.NoError(t, err, "system collection should be accepted")
	if me != nil {
		me.Close()
	}
}

// TestExportNoData verifies that exporting an empty collection succeeds, but
// fails with --assertExists.
func TestExportNoData(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	toolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOptions.Namespace = &options.Namespace{DB: "test", Collection: "mongoexport_no_data_test"}

	me, err := New(Options{
		ToolOptions:         toolOptions,
		OutputFormatOptions: &OutputFormatOptions{Type: "json", JSONFormat: "canonical"},
		InputOptions:        &InputOptions{},
	})
	require.NoError(t, err)
	defer me.Close()
	var buf bytes.Buffer
	_, err = me.Export(&buf)
	assert.NoError(t, err, "export from empty collection should succeed")

	me2, err := New(Options{
		ToolOptions:         toolOptions,
		OutputFormatOptions: &OutputFormatOptions{Type: "json", JSONFormat: "canonical"},
		InputOptions:        &InputOptions{AssertExists: true},
	})
	require.NoError(t, err)
	defer me2.Close()
	_, err = me2.Export(&buf)
	assert.Error(t, err, "export with --assertExists should fail on nonexistent collection")
}
