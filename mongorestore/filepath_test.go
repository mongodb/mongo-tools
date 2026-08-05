// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongorestore/ns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// bump up the verbosity to make checking debug log output possible
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

func newMongoRestore() *MongoRestore {
	renamer, _ := ns.NewRenamer([]string{}, []string{})
	includer, _ := ns.NewMatcher([]string{"*"})
	excluder, _ := ns.NewMatcher([]string{})
	return &MongoRestore{
		manager:      intents.NewIntentManager(),
		InputOptions: &InputOptions{},
		ToolOptions:  &commonOpts.ToolOptions{},
		NSOptions:    &NSOptions{},
		renamer:      renamer,
		includer:     includer,
		excluder:     excluder,
	}
}

func TestCreateAllIntents(t *testing.T) {
	// This tests creates intents based on the test file tree:
	//   testdirs/badfile.txt
	//   testdirs/oplog.bson
	//   testdirs/db1
	//   testdirs/db1/baddir
	//   testdirs/db1/baddir/out.bson
	//   testdirs/db1/c1.bson
	//   testdirs/db1/c1.metadata.json
	//   testdirs/db1/c2.bson
	//   testdirs/db1/c3.bson
	//   testdirs/db1/c3.metadata.json
	//   testdirs/db1/c4.bson
	//   testdirs/db1/c4.metadata.json
	//   testdirs/db2
	//   testdirs/db2/c1.bin
	//   testdirs/db2/c2.txt

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	mr := newMongoRestore()
	var buff bytes.Buffer
	log.SetWriter(&buff)

	ddl, err := newActualPath("testdata/testdirs/")
	require.NoError(t, err, "should resolve the test directory")
	require.NoError(t, mr.CreateAllIntents(ddl), "should create intents for every file in the tree")
	mr.manager.Finalize(intents.Legacy)

	intentsList := []*intents.Intent{
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
	}
	expected := []struct {
		db, c       string
		hasMetadata bool
	}{
		{"db1", "c1", true},
		{"db1", "c2", false},
		{"db1", "c3", true},
		{"db1", "c4", true},
		{"db2", "c1", false},
	}
	for i, want := range expected {
		got := intentsList[i]
		assert.Equal(t, want.db, got.DB, "intent %d should have the expected db", i)
		assert.Equal(t, want.c, got.C, "intent %d should have the expected collection", i)
		assert.NotEqual(t, "", got.Location, "intent %d should have a bson location", i)
		if want.hasMetadata {
			assert.NotEqual(
				t,
				"",
				got.MetadataLocation,
				"intent %d should have a metadata location",
				i,
			)
		} else {
			assert.Equal(t, "", got.MetadataLocation, "intent %d should have no metadata for this file", i)
		}
	}
	require.Nil(t, mr.manager.Pop(), "should have no intents left after popping every expected one")

	logs := buff.String()
	assert.True(t, strings.Contains(logs, "badfile.txt"), "should log the skipped non-bson file")
	assert.True(
		t,
		strings.Contains(logs, "baddir"),
		"should log the skipped directory without bson files",
	)
	assert.True(t, strings.Contains(logs, "c2.txt"), "should log the skipped non-bson file in db2")
}

func TestCreateAllIntentsLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	// This tests creates intents based on the test file tree:
	//   testdata/longcollectionname
	//   testdata/longcollectionname/db1
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.bson
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.metadata.json

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	mr := newMongoRestore()
	var buff bytes.Buffer
	log.SetWriter(&buff)

	ddl, err := newActualPath("testdata/longcollectionname/")
	require.NoError(t, err, "should resolve the test directory")
	require.NoError(t, mr.CreateAllIntents(ddl), "should create intents for every file in the tree")
	mr.manager.Finalize(intents.Legacy)

	i0 := mr.manager.Pop()
	assert.Equal(t, "db1", i0.DB, "intent should belong to db1")
	assert.Equal(t, longCollectionName, i0.C, "intent should have the long collection name")
	assert.NotEqual(t, "", i0.Location, "intent should have a bson location")
	assert.NotEqual(t, "", i0.MetadataLocation, "intent should have a metadata location")
}

func TestCreateIntentsForDB(t *testing.T) {
	// This tests creates intents based on the test file tree:
	//   db1
	//   db1/baddir
	//   db1/baddir/out.bson
	//   db1/c1.bson
	//   db1/c1.metadata.json
	//   db1/c2.bson
	//   db1/c3.bson
	//   db1/c3.metadata.json
	//   db1/c4.bson
	//   db1/c4.metadata.json

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	mr := newMongoRestore()
	var buff bytes.Buffer
	log.SetWriter(&buff)

	ddl, err := newActualPath("testdata/testdirs/db1")
	require.NoError(t, err, "should resolve the test directory")
	require.NoError(
		t,
		mr.CreateIntentsForDB("myDB", ddl),
		"should create intents for every file in the db directory",
	)
	mr.manager.Finalize(intents.Legacy)

	intentsList := []*intents.Intent{
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
	}
	expected := []struct {
		c           string
		hasMetadata bool
	}{
		{"c1", true},
		{"c2", false},
		{"c3", true},
		{"c4", true},
	}
	for i, want := range expected {
		got := intentsList[i]
		assert.Equal(t, want.c, got.C, "intent %d should have the expected collection", i)
		assert.Equal(t, "myDB", got.DB, "intent %d should have the supplied db name", i)
		assert.NotEqual(t, "", got.Location, "intent %d should have a bson location", i)
		if want.hasMetadata {
			assert.NotEqual(
				t,
				"",
				got.MetadataLocation,
				"intent %d should have a metadata location",
				i,
			)
		} else {
			assert.Equal(t, "", got.MetadataLocation, "intent %d should have no metadata for this file", i)
		}
	}
	require.Nil(t, mr.manager.Pop(), "should have no intents left after popping every expected one")

	logs := buff.String()
	assert.True(
		t,
		strings.Contains(logs, "baddir"),
		"should log the skipped directory without bson files",
	)
}

func TestCreateIntentsForDBLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	// This tests creates intents based on the test file tree:
	//   testdata/longcollectionname/db1
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.bson
	//   testdata/longcollectionname/db1/aVery...VeryLongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.metadata.json

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	mr := newMongoRestore()
	var buff bytes.Buffer
	log.SetWriter(&buff)

	ddl, err := newActualPath("testdata/longcollectionname/db1")
	require.NoError(t, err, "should resolve the test directory")
	require.NoError(
		t,
		mr.CreateIntentsForDB("myDB", ddl),
		"should create intents for every file in the db directory",
	)
	mr.manager.Finalize(intents.Legacy)

	i0 := mr.manager.Pop()
	assert.Equal(t, longCollectionName, i0.C, "intent should have the long collection name")
	assert.Equal(t, "myDB", i0.DB, "intent should have the supplied db name")
	assert.NotEqual(t, "", i0.Location, "intent should have a bson location")
	assert.NotEqual(t, "", i0.MetadataLocation, "intent should have a metadata location")
}

func TestCreateIntentsRenamed(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	mr := newMongoRestore()
	mr.renamer, _ = ns.NewRenamer([]string{"db1.*"}, []string{"db4.test.*"})

	ddl, err := newActualPath("testdata/testdirs/")
	require.NoError(t, err, "should resolve the test directory")
	require.NoError(t, mr.CreateAllIntents(ddl), "should create intents for every file in the tree")
	mr.manager.Finalize(intents.Legacy)

	intentsList := []*intents.Intent{
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
		mr.manager.Pop(),
	}
	expected := []struct{ c, db string }{
		{"test.c1", "db4"},
		{"test.c2", "db4"},
		{"test.c3", "db4"},
		{"test.c4", "db4"},
		{"c1", "db2"},
	}
	for i, want := range expected {
		got := intentsList[i]
		assert.Equal(t, want.c, got.C, "intent %d should have the renamed collection", i)
		assert.Equal(t, want.db, got.DB, "intent %d should have the renamed db", i)
	}
	require.Nil(t, mr.manager.Pop(), "should have no intents left after popping every expected one")
}

func TestHandlingBSON(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run(
		"with a target path to a non-truncated bson file instead of a directory",
		func(t *testing.T) {
			mr := newMongoRestore()
			require.NoError(
				t,
				mr.handleBSONInsteadOfDirectory("testdata/testdirs/db1/c2.bson"),
				"should infer the db and collection from the bson path",
			)
			assert.Equal(t, "db1", mr.ToolOptions.DB, "should infer the db from the path")
			assert.Equal(
				t,
				"c2",
				mr.ToolOptions.Collection,
				"should infer the collection from the path",
			)
		},
	)

	t.Run("with a target path to a truncated bson file instead of a directory", func(t *testing.T) {
		mr := newMongoRestore()
		require.NoError(
			t,
			mr.handleBSONInsteadOfDirectory("testdata/longcollectionname/db1/"+longBsonName),
			"should infer the db and collection from the truncated bson path",
		)
		assert.Equal(t, "db1", mr.ToolOptions.DB, "should infer the db from the path")
		assert.Equal(
			t,
			longCollectionName,
			mr.ToolOptions.Collection,
			"should infer the long collection name from the path",
		)
	})

	t.Run("pre-existing collection setting is not overwritten", func(t *testing.T) {
		mr := newMongoRestore()
		mr.ToolOptions.DB = "a"
		mr.ToolOptions.Collection = "b"
		require.NoError(
			t,
			mr.handleBSONInsteadOfDirectory("testdata/testdirs/db1/c1.bson"),
			"should still succeed when settings are pre-existing",
		)
		assert.Equal(t, "a", mr.ToolOptions.DB, "should not overwrite the pre-existing db")
		assert.Equal(
			t,
			"b",
			mr.ToolOptions.Collection,
			"should not overwrite the pre-existing collection",
		)
	})

	t.Run("pre-existing db setting is not overwritten", func(t *testing.T) {
		mr := newMongoRestore()
		mr.ToolOptions.DB = "a"
		require.NoError(
			t,
			mr.handleBSONInsteadOfDirectory("testdata/testdirs/db1/c1.bson"),
			"should still succeed when the db setting is pre-existing",
		)
		assert.Equal(t, "a", mr.ToolOptions.DB, "should not overwrite the pre-existing db")
		assert.Equal(
			t,
			"c1",
			mr.ToolOptions.Collection,
			"should infer the collection from the path",
		)
	})
}

func TestCreateIntentsForCollection(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("on a file without metadata", func(t *testing.T) {
		mr, buff := newCollectionIntentRestore()

		ddl, err := newActualPath(filepath.FromSlash("testdata/testdirs/db1/c2.bson"))
		require.NoError(t, err, "should resolve the bson file")
		require.NoError(
			t,
			mr.CreateIntentForCollection("myDB", "myC", ddl),
			"should create an intent for a file without metadata",
		)
		mr.manager.Finalize(intents.Legacy)

		i0 := mr.manager.Pop()
		require.NotNil(t, i0, "should create one intent")
		assert.Equal(t, "myDB", i0.DB, "intent should have the supplied db")
		assert.Equal(t, "myC", i0.C, "intent should have the supplied collection")
		ddl, err = newActualPath(filepath.FromSlash("testdata/testdirs/db1/c2.bson"))
		require.NoError(t, err, "should resolve the bson file")
		assert.Equal(t, ddl.Path(), i0.Location, "intent should point at the bson file")
		require.Nil(t, mr.manager.Pop(), "should create only one intent")

		assert.Equal(t, "", i0.MetadataLocation, "intent should have no metadata path")
		assert.True(
			t,
			strings.Contains(buff.String(), "without metadata"),
			"should log that the file has no metadata",
		)
	})

	t.Run("on a file with metadata", func(t *testing.T) {
		mr, buff := newCollectionIntentRestore()

		ddl, err := newActualPath(filepath.FromSlash("testdata/testdirs/db1/c1.bson"))
		require.NoError(t, err, "should resolve the bson file")
		require.NoError(
			t,
			mr.CreateIntentForCollection("myDB", "myC", ddl),
			"should create an intent for a file with metadata",
		)
		mr.manager.Finalize(intents.Legacy)

		i0 := mr.manager.Pop()
		require.NotNil(t, i0, "should create one intent")
		assert.Equal(t, "myDB", i0.DB, "intent should have the supplied db")
		assert.Equal(t, "myC", i0.C, "intent should have the supplied collection")
		assert.Equal(
			t,
			filepath.FromSlash("testdata/testdirs/db1/c1.bson"),
			i0.Location,
			"intent should point at the bson file",
		)
		require.Nil(t, mr.manager.Pop(), "should create only one intent")

		assert.Equal(
			t,
			filepath.FromSlash("testdata/testdirs/db1/c1.metadata.json"),
			i0.MetadataLocation,
			"intent should point at the metadata file",
		)
		assert.True(
			t,
			strings.Contains(buff.String(), "found metadata"),
			"should log that the metadata was found",
		)
	})

	t.Run("on a non-existent file", func(t *testing.T) {
		_, err := newActualPath("aaaaaaaaaaaaaa.bson")
		require.Error(t, err, "should reject a non-existent path")
	})

	t.Run("on a directory", func(t *testing.T) {
		mr, _ := newCollectionIntentRestore()

		ddl, err := newActualPath("testdata")
		require.NoError(t, err, "should resolve the directory")
		err = mr.CreateIntentForCollection("myDB", "myC", ddl)
		require.Error(t, err, "should reject a directory")
	})

	t.Run("on a non-bson file", func(t *testing.T) {
		mr, _ := newCollectionIntentRestore()

		ddl, err := newActualPath("testdata/testdirs/db1/c1.metadata.json")
		require.NoError(t, err, "should resolve the file")
		err = mr.CreateIntentForCollection("myDB", "myC", ddl)
		require.Error(t, err, "should reject a non-bson file")
	})
}

func TestCreateIntentForCollectionTimeSeries(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("intent inferred from BSON name", func(t *testing.T) {
		mr, buff, ddl := newTimeSeriesIntentRestore(t)

		err := mr.CreateIntentForCollection(
			mr.ToolOptions.DB,
			mr.ToolOptions.Collection,
			ddl,
		)
		mr.manager.Finalize(intents.Legacy)
		require.NoError(t, err, "should create an intent for the inferred db and collection")

		i0 := mr.manager.Pop()
		require.NotNil(t, i0, "should create one intent")
		assert.Equal(t, mr.ToolOptions.DB, i0.DB, "intent should have the inferred db")
		assert.Equal(
			t,
			"foo_ts",
			i0.C,
			"intent should strip the system.buckets prefix from the collection",
		)
		assert.Equal(
			t,
			filepath.FromSlash(ddl.Path()),
			i0.Location,
			"intent should point at the bson file",
		)
		require.Nil(t, mr.manager.Pop(), "should create only one intent")

		assert.Equal(
			t,
			filepath.FromSlash(
				"testdata/timeseries_tests/ts_dump/timeseries_test/foo_ts.metadata.json",
			),
			i0.MetadataLocation,
			"intent should point at the metadata file",
		)
		assert.True(
			t,
			strings.Contains(buff.String(), "found metadata"),
			"should log that the metadata was found",
		)
	})

	t.Run(
		"intent correct when input db and collection already contain the system.buckets prefix",
		func(t *testing.T) {
			mr, buff, ddl := newTimeSeriesIntentRestore(t)

			err := mr.CreateIntentForCollection("myDB", "system.buckets.myC", ddl)
			mr.manager.Finalize(intents.Legacy)
			require.NoError(t, err, "should create an intent when the prefix is already present")

			i0 := mr.manager.Pop()
			require.NotNil(t, i0, "should create one intent")
			assert.Equal(t, "myDB", i0.DB, "intent should have the supplied db")
			assert.Equal(
				t,
				"myC",
				i0.C,
				"intent should strip the system.buckets prefix from the collection",
			)
			assert.Equal(t, ddl.Path(), i0.Location, "intent should point at the bson file")
			require.Nil(t, mr.manager.Pop(), "should create only one intent")

			assert.Equal(
				t,
				filepath.FromSlash(
					"testdata/timeseries_tests/ts_dump/timeseries_test/foo_ts.metadata.json",
				),
				i0.MetadataLocation,
				"intent should point at the metadata file",
			)
			assert.True(
				t,
				strings.Contains(buff.String(), "found metadata"),
				"should log that the metadata was found",
			)
		},
	)
}

// newTimeSeriesIntentRestore builds a fresh MongoRestore whose ToolOptions
// already reflect a system.buckets bson path, matching the shared setup that
// GoConvey re-ran for each of this test's two independent scenarios.
func newTimeSeriesIntentRestore(t *testing.T) (*MongoRestore, *bytes.Buffer, *actualPath) {
	t.Helper()

	mr, buff := newCollectionIntentRestore()

	ddl, err := newActualPath(
		filepath.FromSlash(
			"testdata/timeseries_tests/ts_dump/timeseries_test/system.buckets.foo_ts.bson",
		),
	)
	require.NoError(t, err, "should resolve the system.buckets bson file")
	mr.ToolOptions.Namespace = &commonOpts.Namespace{}

	require.NoError(
		t,
		mr.handleBSONInsteadOfDirectory(ddl.Path()),
		"should infer the db and collection from the system.buckets path",
	)
	require.Equal(t, "timeseries_test", mr.ToolOptions.DB, "should infer the db from the path")
	require.Equal(
		t,
		"system.buckets.foo_ts",
		mr.ToolOptions.Collection,
		"should infer the system.buckets collection from the path",
	)

	return mr, buff, ddl
}

func TestCreateIntentsForLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("on a truncated bson file without metadata", func(t *testing.T) {
		mr, _ := newCollectionIntentRestore()

		ddl, err := newActualPath(
			filepath.FromSlash("testdata/longcollectionname/" + longInvalidBson),
		)
		require.NoError(t, err, "should resolve the truncated bson file")
		err = mr.CreateIntentForCollection("myDB", "myC", ddl)
		require.Error(t, err, "should reject a truncated bson file without metadata")
	})

	t.Run("on a truncated bson file with metadata", func(t *testing.T) {
		mr, buff := newCollectionIntentRestore()

		ddl, err := newActualPath(
			filepath.FromSlash("testdata/longcollectionname/db1/" + longBsonName),
		)
		require.NoError(t, err, "should resolve the truncated bson file")
		require.NoError(
			t,
			mr.CreateIntentForCollection("myDB", "myC", ddl),
			"should create an intent for a truncated bson file with metadata",
		)
		mr.manager.Finalize(intents.Legacy)

		i0 := mr.manager.Pop()
		require.NotNil(t, i0, "should create one intent")
		assert.Equal(t, "myDB", i0.DB, "intent should have the supplied db")
		assert.Equal(t, "myC", i0.C, "intent should have the supplied collection")
		assert.Equal(
			t,
			filepath.FromSlash("testdata/longcollectionname/db1/"+longBsonName),
			i0.Location,
			"intent should point at the truncated bson file",
		)
		require.Nil(t, mr.manager.Pop(), "should create only one intent")

		assert.Equal(
			t,
			filepath.FromSlash(
				"testdata/longcollectionname/db1/"+longMetadataName,
			),
			i0.MetadataLocation,
			"intent should point at the metadata file",
		)
		assert.True(
			t,
			strings.Contains(buff.String(), "found metadata"),
			"should log that the metadata was found",
		)
	})
}

func newCollectionIntentRestore() (*MongoRestore, *bytes.Buffer) {
	var buff bytes.Buffer
	mr := &MongoRestore{
		manager:      intents.NewIntentManager(),
		ToolOptions:  &commonOpts.ToolOptions{},
		InputOptions: &InputOptions{},
	}
	log.SetWriter(&buff)
	return mr, &buff
}
