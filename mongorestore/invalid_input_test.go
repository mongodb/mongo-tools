// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestRestoreInvalidInput consolidates the mongorestore invalid-input and
// error-path coverage that previously lived in the qa-tests JS suite
// (bad_options.js, missing_dump.js, invalid_dump_target.js, malformed_bson.js,
// malformed_metadata.js, invalid_metadata.js, blank_collection_bson.js,
// blank_db.js, objcheck_valid_bson.js, oplog_replay_no_oplog.js). Each case
// asserts on the error returned by option validation or by Restore(), rather
// than on a process exit code.
//
// bad_options.js's invalid-verbosity case (-v torvalds) is intentionally not
// converted: verbosity parsing lives in the shared common/options package, not
// in mongorestore, so it is out of scope for these restore-specific tests.
func TestRestoreInvalidInput(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	t.Run("--objcheck with --noobjcheck is rejected", testRestoreObjcheckWithNoobjcheck)
	t.Run("negative write concern is rejected", testRestoreNegativeWriteConcern)
	t.Run("malformed --oplogLimit timestamp is rejected", testRestoreMalformedOplogLimit)
	t.Run("invalid db name is rejected", testRestoreInvalidDBName)
	t.Run("invalid collection name is rejected", testRestoreInvalidCollectionName)
	t.Run("missing dump directory errors", testRestoreMissingDumpDirectory)
	t.Run("missing dump directory with --db errors", testRestoreMissingDumpDirectoryWithDB)
	t.Run("missing bson file with --collection errors", testRestoreMissingBSONFile)
	t.Run("file instead of directory errors", testRestoreFileInsteadOfDirectory)
	t.Run("file instead of directory with --db errors", testRestoreFileInsteadOfDBDirectory)
	t.Run(
		"directory instead of bson file with --collection errors",
		testRestoreDirectoryInsteadOfBSONFile,
	)
	t.Run("malformed bson file errors", testRestoreMalformedBSON)
	t.Run("malformed metadata file errors", testRestoreMalformedMetadata)
	t.Run("invalid index in metadata errors", testRestoreInvalidIndexMetadata)
	t.Run(
		"blank collection bson without metadata inserts nothing",
		testRestoreBlankCollectionBSONWithoutMetadata,
	)
	t.Run(
		"blank collection bson with metadata inserts nothing",
		testRestoreBlankCollectionBSONWithMetadata,
	)
	t.Run("blank db directory succeeds", testRestoreBlankDBDirectory)
	t.Run("objcheck succeeds on valid bson", testRestoreObjcheckValidBSON)
	t.Run("oplogReplay with no oplog file errors", testRestoreOplogReplayNoOplogFile)
}

// --noobjcheck was removed when the tools were rewritten in Go, so passing it
// alongside --objcheck is now rejected as an unknown flag. The JS test's intent
// (the two flags cannot be combined) still holds.
func testRestoreObjcheckWithNoobjcheck(t *testing.T) {
	_, err := getRestoreWithArgs(ObjcheckOption, "--noobjcheck", t.TempDir())
	require.ErrorContains(
		t, err, "noobjcheck",
		"combining --objcheck with the removed --noobjcheck flag is rejected",
	)
}

func testRestoreNegativeWriteConcern(t *testing.T) {
	_, err := getRestoreWithArgs(WriteConcernOption+"=-1", t.TempDir())
	require.ErrorContains(
		t, err, "invalid 'w' argument",
		"a negative --writeConcern value is rejected",
	)
}

func testRestoreMalformedOplogLimit(t *testing.T) {
	restore, err := getRestoreWithArgs(
		OplogReplayOption,
		OplogLimitOption,
		"xxx",
		t.TempDir(),
	)
	require.NoError(t, err, "building the restore instance")
	defer restore.Close()
	require.ErrorContains(
		t, restore.ParseAndValidateOptions(),
		"error parsing timestamp argument to --oplogLimit",
		"a non-timestamp --oplogLimit value is rejected",
	)
}

func testRestoreInvalidDBName(t *testing.T) {
	restore, err := getRestoreWithArgs(DBOption, "billy.crystal", t.TempDir())
	require.NoError(t, err, "building the restore instance")
	defer restore.Close()
	require.ErrorContains(
		t, restore.ParseAndValidateOptions(), "invalid db name",
		"a db name containing an illegal character is rejected",
	)
}

func testRestoreInvalidCollectionName(t *testing.T) {
	bsonFile := filepath.Join(t.TempDir(), "coll.bson")
	writeBSONCollectionFile(t, bsonFile)
	restore, err := getRestoreWithArgs(
		DBOption,
		"test",
		CollectionOption,
		"$money",
		bsonFile,
	)
	require.NoError(t, err, "building the restore instance")
	defer restore.Close()
	require.ErrorContains(
		t, restore.ParseAndValidateOptions(), "invalid collection name",
		"a collection name containing an illegal character is rejected",
	)
}

func testRestoreMissingDumpDirectory(t *testing.T) {
	result := restoreFromArgs(t, missingPath(t))
	require.ErrorContains(
		t, result.Err, "invalid",
		"restoring from a nonexistent directory errors",
	)
}

func testRestoreMissingDumpDirectoryWithDB(t *testing.T) {
	result := restoreFromArgs(t, DBOption, "test", missingPath(t))
	require.ErrorContains(
		t, result.Err, "invalid",
		"restoring from a nonexistent directory with --db errors",
	)
}

func testRestoreMissingBSONFile(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "missing.bson")
	result := restoreFromArgs(t, DBOption, "test", CollectionOption, "data", missingFile)
	require.ErrorContains(
		t, result.Err, "invalid",
		"restoring from a nonexistent bson file errors",
	)
}

func testRestoreFileInsteadOfDirectory(t *testing.T) {
	result := restoreFromArgs(t, writeNonDumpFile(t))
	require.ErrorContains(
		t, result.Err, "does not have .bson extension",
		"restoring with a file where a directory is expected errors",
	)
}

func testRestoreFileInsteadOfDBDirectory(t *testing.T) {
	result := restoreFromArgs(t, DBOption, "test", writeNonDumpFile(t))
	require.ErrorContains(
		t, result.Err, "does not have .bson extension",
		"restoring with a file where a db directory is expected errors",
	)
}

func testRestoreDirectoryInsteadOfBSONFile(t *testing.T) {
	result := restoreFromArgs(t, DBOption, "test", CollectionOption, "blank", t.TempDir())
	require.ErrorContains(
		t, result.Err, "is a directory, not a bson file",
		"restoring with a directory where a bson file is expected errors",
	)
}

func testRestoreMalformedBSON(t *testing.T) {
	bsonFile := filepath.Join(t.TempDir(), "malformed_coll.bson")
	require.NoError(
		t, os.WriteFile(bsonFile, []byte("this is not valid bson at all"), 0644),
		"writing malformed bson fixture",
	)
	result := restoreFromArgs(
		t,
		DBOption,
		"dbOne",
		CollectionOption,
		"malformed_coll",
		bsonFile,
	)
	require.ErrorContains(
		t, result.Err, "reading bson input",
		"restoring a malformed bson file errors",
	)
}

func testRestoreMalformedMetadata(t *testing.T) {
	dir := t.TempDir()
	bsonFile := filepath.Join(dir, "coll.bson")
	writeBSONCollectionFile(t, bsonFile, bson.M{"_id": 1})
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "coll.metadata.json"),
			[]byte("{ this is not json"),
			0644,
		),
		"writing malformed metadata fixture",
	)
	result := restoreFromArgs(t, DBOption, "dbOne", CollectionOption, "coll", bsonFile)
	require.ErrorContains(
		t, result.Err, "error parsing metadata",
		"restoring with a syntactically invalid metadata file errors",
	)
}

func testRestoreInvalidIndexMetadata(t *testing.T) {
	dir := t.TempDir()
	bsonFile := filepath.Join(dir, "coll.bson")
	writeBSONCollectionFile(t, bsonFile, bson.M{"_id": 1})
	// Well-formed JSON, but an index spec that the server will reject (empty key
	// document).
	metadata := `{"options":{},"indexes":[{"v":2,"key":{},"name":"bad_index"}]}`
	require.NoError(
		t, os.WriteFile(filepath.Join(dir, "coll.metadata.json"), []byte(metadata), 0644),
		"writing invalid-index metadata fixture",
	)
	result := restoreFromArgs(t, DBOption, "dbOne", CollectionOption, "coll", bsonFile)
	require.Error(t, result.Err, "restoring metadata with an invalid index spec errors")
}

func testRestoreBlankCollectionBSONWithoutMetadata(t *testing.T) {
	client := testClientWithDroppedDB(t, "test")

	bsonFile := filepath.Join(t.TempDir(), "blank.bson")
	writeBSONCollectionFile(t, bsonFile)
	result := restoreFromArgs(t, DBOption, "test", CollectionOption, "blank", bsonFile)
	require.NoError(t, result.Err, "restoring a blank collection file succeeds")

	count, err := client.Database("test").
		Collection("blank").
		CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "counting restored documents")
	assert.EqualValues(t, 0, count, "a blank collection file inserts nothing")
}

func testRestoreBlankCollectionBSONWithMetadata(t *testing.T) {
	client := testClientWithDroppedDB(t, "test")

	dir := t.TempDir()
	bsonFile := filepath.Join(dir, "blank.bson")
	writeBSONCollectionFile(t, bsonFile)
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "blank.metadata.json"),
			[]byte(`{"options":{},"indexes":[]}`),
			0644,
		),
		"writing empty metadata fixture",
	)
	result := restoreFromArgs(t, DBOption, "test", CollectionOption, "blank", bsonFile)
	require.NoError(
		t,
		result.Err,
		"restoring a blank collection file with metadata succeeds",
	)

	count, err := client.Database("test").
		Collection("blank").
		CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "counting restored documents")
	assert.EqualValues(t, 0, count, "a blank collection file with metadata inserts nothing")
}

func testRestoreBlankDBDirectory(t *testing.T) {
	result := restoreFromArgs(t, DBOption, "test", t.TempDir())
	require.NoError(t, result.Err, "restoring from an empty db directory succeeds")
	assert.EqualValues(t, 0, result.Successes, "an empty db directory inserts nothing")
}

func testRestoreObjcheckValidBSON(t *testing.T) {
	client := testClientWithDroppedDB(t, "test")
	require.NoError(
		t,
		client.Database("test").Collection("coll").Drop(t.Context()),
		"dropping target collection",
	)

	const numDocs = 50
	docs := make([]bson.M, numDocs)
	for i := range docs {
		docs[i] = bson.M{"_id": i}
	}
	bsonFile := filepath.Join(t.TempDir(), "coll.bson")
	writeBSONCollectionFile(t, bsonFile, docs...)

	result := restoreFromArgs(
		t,
		ObjcheckOption,
		DBOption,
		"test",
		CollectionOption,
		"coll",
		bsonFile,
	)
	require.NoError(t, result.Err, "restoring valid bson with --objcheck succeeds")

	count, err := client.Database("test").
		Collection("coll").
		CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "counting restored documents")
	assert.EqualValues(t, numDocs, count, "all documents are restored with --objcheck")
}

func testRestoreOplogReplayNoOplogFile(t *testing.T) {
	result := restoreFromArgs(t, OplogReplayOption, t.TempDir())
	require.ErrorContains(
		t, result.Err, "no oplog file to replay",
		"--oplogReplay against a dump with no oplog.bson errors",
	)
}

// restoreFromArgs builds a MongoRestore from the given args, runs it, and
// returns the result. The instance is closed when the test finishes.
func restoreFromArgs(t *testing.T, args ...string) Result {
	t.Helper()
	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "building the restore instance")
	t.Cleanup(restore.Close)
	return restore.Restore()
}

// writeBSONCollectionFile writes the given documents to path in the
// concatenated-BSON format that mongodump produces for a collection (an empty
// docs list produces a zero-byte file, i.e. a blank collection).
func writeBSONCollectionFile(t *testing.T, path string, docs ...bson.M) {
	t.Helper()
	var buf []byte
	for _, doc := range docs {
		marshaled, err := bson.Marshal(doc)
		require.NoError(t, err, "marshaling fixture document")
		buf = append(buf, marshaled...)
	}
	require.NoError(t, os.WriteFile(path, buf, 0644), "writing bson fixture file")
}

// missingPath returns a path inside a temp directory that does not exist.
func missingPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "does-not-exist")
}

// writeNonDumpFile writes a plain (non-.bson) file and returns its path, for
// cases that point mongorestore at a file where a directory is expected.
func writeNonDumpFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "README")
	require.NoError(
		t,
		os.WriteFile(path, []byte("not a dump"), 0644),
		"writing file target",
	)
	return path
}

// testClientWithDroppedDB returns a bare session, dropping dbName when the test
// finishes.
func testClientWithDroppedDB(t *testing.T, dbName string) *mongo.Client {
	t.Helper()
	client, err := testutil.GetBareSession()
	require.NoError(t, err, "connecting to the test server")
	t.Cleanup(func() { _ = client.Database(dbName).Drop(t.Context()) })
	return client
}
