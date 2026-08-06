// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/idx"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

func TestTimestampStringParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		in          string
		expectedTS  bson.Timestamp
		expectError bool
	}{
		{"123:456", bson.Timestamp{T: 123, I: 456}, false},
		{"123", bson.Timestamp{T: 123, I: 0}, false},
		{"123:", bson.Timestamp{T: 123, I: 0}, false},
		{"123.123", bson.Timestamp{}, true},
		{":", bson.Timestamp{}, true},
		{"1:1:1", bson.Timestamp{}, true},
		{"cats", bson.Timestamp{}, true},
		{"", bson.Timestamp{}, true},
	}

	for _, tc := range cases {
		ts, err := ParseTimestampFlag(tc.in)
		if tc.expectError {
			assert.Error(t, err, "should reject timestamp string %q", tc.in)
		} else {
			assert.NoError(t, err, "should parse timestamp string %q", tc.in)
		}
		assert.Equal(t, tc.expectedTS, ts, "should parse %q into the expected timestamp", tc.in)
	}
}

func TestValidOplogLimitChecking(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("with oplogLimit of 5:0", func(t *testing.T) {
		mr := &MongoRestore{
			oplogLimit: bson.Timestamp{T: 5, I: 0},
		}

		cases := []struct {
			ts       bson.Timestamp
			expected bool
		}{
			{bson.Timestamp{T: 1000, I: 0}, false},
			{bson.Timestamp{T: 5, I: 1}, false},
			{bson.Timestamp{T: 5, I: 0}, false},
			{bson.Timestamp{T: 4, I: 9}, true},
			{bson.Timestamp{T: 4, I: 0}, true},
			{bson.Timestamp{T: 0, I: 1}, true},
		}
		for _, tc := range cases {
			assert.Equal(
				t,
				tc.expected,
				mr.TimestampBeforeLimit(tc.ts),
				"should report ts=%v as before-limit=%v",
				tc.ts,
				tc.expected,
			)
		}
	})

	t.Run("with no oplogLimit", func(t *testing.T) {
		mr := &MongoRestore{}

		cases := []struct {
			ts       bson.Timestamp
			expected bool
		}{
			{bson.Timestamp{T: 1000, I: 0}, true},
			{bson.Timestamp{T: 5, I: 1}, true},
			{bson.Timestamp{T: 5, I: 0}, true},
		}
		for _, tc := range cases {
			assert.Equal(
				t,
				tc.expected,
				mr.TimestampBeforeLimit(tc.ts),
				"should report ts=%v as before-limit=%v",
				tc.ts,
				tc.expected,
			)
		}
	})
}

func TestOplogRestore(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	fcv := testutil.GetFCV(session)
	var shouldPreserveUUID bool
	if cmp, err := testutil.CompareFCV(fcv, "3.6"); err != nil || cmp >= 0 {
		shouldPreserveUUID = true
	}

	args := []string{
		DirectoryOption, "testdata/oplogdump",
		OplogReplayOption,
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
		DropOption,
	}
	if shouldPreserveUUID {
		args = append(args, PreserveUUIDOption)
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()
	c1 := session.Database("db1").Collection("c1")
	err = c1.Drop(t.Context())
	require.NoError(t, err, "should drop the target collection")

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")
	require.EqualValues(t, 0, result.Failures, "should restore with no failures")

	// Verify restoration
	count, err := c1.CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "should count the restored documents")
	require.EqualValues(t, 10, count, "should restore every document from the oplog")
	err = session.Disconnect(t.Context())
	require.NoError(t, err, "should disconnect from the server")
}

func TestOplogRestoreWithDuplicateIndexKeys(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	args := []string{
		DirectoryOption, "testdata/duplicate_index_key_with_oplog",
		OplogReplayOption,
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()
	coll := session.Database("test").Collection("foo")

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")
	require.EqualValues(t, 0, result.Failures, "should restore with no failures")

	// Verify restoration
	count, err := coll.CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "should count the restored documents")
	require.EqualValues(t, 1, count, "should restore the single document")
	err = session.Disconnect(t.Context())
	require.NoError(t, err, "should disconnect from the server")
}

func TestOplogRestoreUpdatesIndexCatalog(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	//nolint:errcheck
	defer session.Disconnect(t.Context())

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must get a session provider")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "should get the server version")

	t.Run("index drop in oplog should delete it from indexCatalog", func(t *testing.T) {
		args := []string{
			DirectoryOption, "testdata/coll_with_index",
			OplogReplayOption,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			DropOption,
			OplogFileOption, "testdata/oplogs/bson/drop_index.bson",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		require.EqualValues(t, 0, result.Failures, "should restore with no failures")

		coll := session.Database("test").Collection("foo")

		ctx := t.Context()
		// Verify restoration
		count, err := coll.CountDocuments(ctx, bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(t, 1, count, "should restore the single document")

		indexCursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err, "should list the collection's indexes")

		defer indexCursor.Close(ctx)

		indexCount := 0
		for indexCursor.Next(ctx) {
			indexCount++
		}

		assert.Equal(
			t,
			1,
			indexCount,
			"should have only the default index after the dropped index is replayed",
		)
	})

	t.Run("collection drop in oplog should delete indexes from indexCatalog", func(t *testing.T) {
		args := []string{
			DirectoryOption, "testdata/coll_with_index",
			OplogReplayOption,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			DropOption,
			OplogFileOption, "testdata/oplogs/bson/drop_collection.bson",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		require.EqualValues(t, 0, result.Failures, "should restore with no failures")

		coll := session.Database("test").Collection("foo")

		ctx := t.Context()
		// Verify restoration
		count, err := coll.CountDocuments(ctx, bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(
			t,
			0,
			count,
			"should have no documents after the collection drop is replayed",
		)

		indexCursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err, "should list the collection's indexes")

		defer indexCursor.Close(ctx)

		indexCount := 0
		for indexCursor.Next(ctx) {
			indexCount++
		}

		assert.Equal(
			t,
			0,
			indexCount,
			"should have no indexes after the collection drop is replayed",
		)
	})

	// This will fail with pre-5.3.0 versions because of SERVER-62759.
	if serverVersion.GTE(db.Version{5, 3, 0}) {
		t.Run("db drop in oplog should delete indexes from indexCatalog", func(t *testing.T) {
			args := []string{
				DirectoryOption, "testdata/coll_with_index",
				OplogReplayOption,
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
				DropOption,
				OplogFileOption, "testdata/oplogs/bson/drop_db.bson",
			}

			restore, err := getRestoreWithArgs(args...)
			require.NoError(t, err, "should build a restore instance")
			defer restore.Close()

			// Run mongorestore
			result := restore.Restore()
			require.NoError(t, result.Err, "should restore without error")
			require.EqualValues(t, 0, result.Failures, "should restore with no failures")

			coll := session.Database("test").Collection("foo")

			ctx := t.Context()
			// Verify restoration
			count, err := coll.CountDocuments(ctx, bson.M{})
			require.NoError(t, err, "should count the restored documents")
			require.EqualValues(
				t,
				0,
				count,
				"should have no documents after the db drop is replayed",
			)

			indexCursor, err := coll.Indexes().List(ctx)
			require.NoError(t, err, "should list the collection's indexes")

			defer indexCursor.Close(ctx)

			indexCount := 0
			for indexCursor.Next(ctx) {
				indexCount++
			}

			assert.Equal(t, 0, indexCount, "should have no indexes after the db drop is replayed")
		})
	}

	t.Run("create indexes should update indexCatalog", func(t *testing.T) {
		args := []string{
			DirectoryOption, "testdata/coll_with_ttl_index",
			OplogReplayOption,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			DropOption,
			OplogFileOption, "testdata/oplogs/bson/create_index.bson",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		require.EqualValues(t, 0, result.Failures, "should restore with no failures")

		coll := session.Database("test").Collection("foo")

		ctx := t.Context()
		// Verify restoration
		count, err := coll.CountDocuments(ctx, bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(t, 1, count, "should restore the single document")

		indexCursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err, "should list the collection's indexes")

		defer indexCursor.Close(ctx)

		indexCount := 0
		for indexCursor.Next(ctx) {
			indexCount++
		}

		assert.Equal(t, 2, indexCount, "should have the default index plus the created index")
	})

	t.Run("collMod should edit index in indexCatalog", func(t *testing.T) {
		args := []string{
			DirectoryOption, "testdata/coll_with_ttl_index",
			OplogReplayOption,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			DropOption,
			OplogFileOption, "testdata/oplogs/bson/collMod.bson",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		require.EqualValues(t, 0, result.Failures, "should restore with no failures")

		coll := session.Database("test").Collection("foo")

		ctx := t.Context()
		// Verify restoration
		count, err := coll.CountDocuments(ctx, bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(t, 1, count, "should restore the single document")

		indexCursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err, "should list the collection's indexes")

		defer indexCursor.Close(ctx)

		var indexDoc idx.IndexDocument

		for indexCursor.Next(ctx) {
			err = indexCursor.Decode(&indexDoc)
			require.NoError(t, err, "should decode each index document")
			if indexDoc.Options["name"] == "f_1" {
				assert.EqualValues(
					t,
					3600,
					indexDoc.Options["expireAfterSeconds"],
					"should apply the collMod's expireAfterSeconds",
				)
			}
		}
	})

	t.Run("collMod should edit hidden field in index in indexCatalog", func(t *testing.T) {
		fcv := testutil.GetFCV(session)
		if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
			t.Skip("Requires server with FCV 4.4 or later")
		}

		args := []string{
			DirectoryOption, "testdata/coll_with_ttl_index",
			OplogReplayOption,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			DropOption,
			OplogFileOption, "testdata/oplogs/bson/collMod_with_hidden.bson",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		require.EqualValues(t, 0, result.Failures, "should restore with no failures")

		coll := session.Database("test").Collection("foo")

		ctx := t.Context()
		// Verify restoration
		count, err := coll.CountDocuments(ctx, bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(t, 1, count, "should restore the single document")

		indexCursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err, "should list the collection's indexes")

		defer indexCursor.Close(ctx)

		var indexDoc idx.IndexDocument

		for indexCursor.Next(ctx) {
			err = indexCursor.Decode(&indexDoc)
			require.NoError(t, err, "should decode each index document")
			if indexDoc.Options["name"] == "f_1" {
				assert.EqualValues(
					t,
					3600,
					indexDoc.Options["expireAfterSeconds"],
					"should apply the collMod's expireAfterSeconds",
				)
				assert.EqualValues(
					t,
					true,
					indexDoc.Options["hidden"],
					"should apply the collMod's hidden flag",
				)
			}
		}
	})
}

func TestOplogRestoreMaxDocumentSize(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	fcv := testutil.GetFCV(session)
	var shouldPreserveUUID bool
	if cmp, err := testutil.CompareFCV(fcv, "3.6"); err != nil || cmp >= 0 {
		shouldPreserveUUID = true
	}

	c1 := session.Database("db1").Collection("c1")
	err = c1.Drop(t.Context())
	require.NoError(t, err, "should drop db1.c1")

	// Generate an oplog document and verify that size exceeds 16 MiB.
	oplogBytes, err := generateOplogWith16MiBDocument()
	require.NoError(t, err, "should generate a 16 MiB oplog document")
	require.Greater(
		t,
		len(oplogBytes),
		db.MaxBSONSize,
		"should generate a document larger than the max BSON size",
	)

	// Temporarily write the oplog document to testdata/oplogdumpmaxsize/oplog.bson
	err = os.WriteFile("testdata/oplogdumpmaxsize/oplog.bson", oplogBytes, 0644)
	require.NoError(t, err, "should write the generated oplog document to disk")
	defer os.Remove("testdata/oplogdumpmaxsize/oplog.bson")

	args := []string{
		DirectoryOption, "testdata/oplogdumpmaxsize",
		OplogReplayOption,
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
		DropOption,
	}
	if shouldPreserveUUID {
		args = append(args, PreserveUUIDOption)
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	// Make sure to drop the 16 MiB collection before disconnecting.
	//
	//nolint:errcheck
	defer session.Disconnect(t.Context())
	//nolint:errcheck
	defer c1.Drop(t.Context())

	// Run mongorestore.
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")
	require.EqualValues(t, 0, result.Failures, "should restore with no failures")

	// Verify restoration (5 docs in c1.bson + 1 doc in oplog.bson).
	count, err := c1.CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "should count the restored documents")
	require.EqualValues(t, 6, count, "should restore every document including the oversized one")
}

// Generates an oplog document that is greater than 16 MiB but less than 16 MiB + 16 KiB.
// Returns the oplog document's raw bytes.
func generateOplogWith16MiBDocument() ([]byte, error) {

	// Generate a document of the form {_id: X, key: Y} where the total document size
	// is equal to 16 MiB. Generates a long string for Y in order to reach 16 MiB.
	//
	// Here's a breakdown of bytes in the document:
	//
	// 4 bytes = document length
	// 1 byte = element type (ObjectID = \x07)
	// 4 bytes = key name ("_id" + \x00)
	// 12 bytes = ObjectID value
	// 1 byte = element type (string = \x02)
	// 4 bytes = key name ("key" + \x00)
	// 4 bytes = string length
	// X bytes = string of length X bytes
	// 1 byte = \x00
	// 1 byte = \x00
	//
	// Therefore the string length should be: 1024*1024*16 - 32

	size := 1024*1024*16 - 32

	idx, rawdoc := bsoncore.AppendDocumentStart(nil)
	rawdoc = bsoncore.AppendObjectIDElement(rawdoc, "_id", bson.NewObjectID())
	rawdoc = bsoncore.AppendStringElement(rawdoc, "key", strings.Repeat("A", size))
	rawdoc, _ = bsoncore.AppendDocumentEnd(rawdoc, idx)

	// Creating the oplog document with the above 16 MiB document will allow
	// the oplog document to exceed 16 MiB with the additional metadata.
	var doc bson.D
	if err := bson.Unmarshal(rawdoc, &doc); err != nil {
		return nil, err
	}
	oplog := db.Oplog{
		Version:   2,
		Operation: "i",
		Namespace: "db1.c1",
		Object:    doc,
	}

	return bson.Marshal(oplog)
}

func TestOplogRestoreTools2002(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	args := []string{
		DirectoryOption, "testdata/tools-2002",
		OplogReplayOption,
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
		DropOption,
	}
	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")
	require.EqualValues(t, 0, result.Failures, "should restore with no failures")
}

type testTable struct {
	ns     string
	output bool
}

func TestShouldIgnoreNamespace(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	tests := []testTable{
		{
			ns:     "test.system",
			output: false,
		},
		{
			ns:     "test.system.nonsense",
			output: false,
		},
		{
			ns:     "config.system.sessions",
			output: true,
		},
		{
			ns:     "config.system.indexBuilds",
			output: true,
		},
		{
			ns:     "config.system.preimages",
			output: true,
		},
		{
			ns:     "config.transactions",
			output: true,
		},
		{
			ns:     "config.transaction_coordinators",
			output: true,
		},
		{
			ns:     "config.system.sharding_ddl_coordinators",
			output: true,
		},
		{
			ns:     "config.image_collection",
			output: true,
		},
		{
			ns:     "config.mongos",
			output: true,
		},
		{
			ns:     "test.system.js",
			output: false,
		},
		{
			ns:     "test.test",
			output: false,
		},
		{
			ns:     "config.cache.any",
			output: true,
		},
	}
	for _, testVals := range tests {
		assert.Equal(
			t,
			testVals.output,
			shouldIgnoreNamespace(testVals.ns),
			"should report %s as ignored=%v",
			testVals.ns,
			testVals.output,
		)
	}
}

func TestOplogRestoreVectoredInsert(t *testing.T) {
	testOplogRestoreVectoredInsert(t, true)
	testOplogRestoreVectoredInsert(t, false)
}

func testOplogRestoreVectoredInsert(t *testing.T, linked bool) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ctx := t.Context()

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	//nolint:errcheck
	defer session.Disconnect(ctx)

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "8.0"); err != nil || cmp < 0 {
		assert.NoError(t, err, "should get the server's FCV")
		t.Skipf("Requires server with FCV 8.0 or later; found %v", fcv)
	}

	// Prepare the test by creating the necessary collection.
	require.NoError(t, session.Database("mongodump_test_db").Drop(ctx))
	t.Cleanup(func() { _ = session.Database("mongodump_test_db").Drop(t.Context()) })
	require.NoError(t, session.Database("mongodump_test_db").CreateCollection(ctx, "coll1"))

	oplogFileName := "testdata/oplogs/bson/vectored_insert.bson"
	if linked {
		oplogFileName = "testdata/oplogs/bson/linked_vectored_inserts.bson"
	}

	args := []string{
		DirectoryOption, "testdata/coll_without_index",
		OplogReplayOption,
		DropOption,
		OplogFileOption, oplogFileName,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err)
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err)
	require.Equal(t, int64(0), result.Failures)

	coll := session.Database("mongodump_test_db").Collection("coll1")
	//defer require.NoError(t, coll.Drop(ctx))

	// Verify restoration
	cursor, err := coll.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"_id", 1}}))
	require.NoError(t, err)
	defer cursor.Close(ctx)

	expectedDocs := []bson.D{
		{{"_id", 100}, {"a", 1}},
		{{"_id", 200}, {"a", 2}},
	}
	if linked {
		expectedDocs = []bson.D{
			{{"_id", 300}, {"a", 3}},
			{{"_id", 400}, {"a", 4}},
			{{"_id", 500}, {"a", 5}},
			{{"_id", 600}, {"a", 6}},
			{{"_id", 700}, {"a", 7}},
		}
	}

	i := 0
	for cursor.Next(ctx) {
		fmt.Println(cursor.Current)
		require.Less(t, i, len(expectedDocs))
		expectedDocRaw, marshalErr := bson.Marshal(expectedDocs[i])
		require.NoError(t, marshalErr)
		require.Equal(t, bson.Raw(expectedDocRaw), cursor.Current)
		i++
	}
	require.Equal(t, len(expectedDocs), i)
}

func TestOplogRestoreCollModIndexUniqueness(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ctx := t.Context()

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	//nolint:errcheck
	defer session.Disconnect(ctx)

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "6.0"); err != nil || cmp < 0 {
		assert.NoError(t, err, "should get the server's FCV")
		t.Skipf("Requires server with FCV 6.0 or later; found %v", fcv)
	}

	// Prepare the test by creating the necessary collection.
	require.NoError(t, session.Database("mongodump_test_db").Drop(ctx))
	t.Cleanup(func() { _ = session.Database("mongodump_test_db").Drop(t.Context()) })
	require.NoError(t, session.Database("mongodump_test_db").CreateCollection(ctx, "coll1"))

	oplogFileName := "testdata/oplogs/bson/collMod_indexUniqueness.bson"

	args := []string{
		DirectoryOption, "testdata/coll_without_index",
		OplogReplayOption,
		DropOption,
		OplogFileOption, oplogFileName,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err)
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err)
	require.Equal(t, int64(0), result.Failures)

	db := session.Database("mongodump_test_db")

	cursor, err := db.RunCommandCursor(ctx, bson.D{
		{"listIndexes", "coll1"},
	})
	require.NoError(t, err)

	var indexSpecs []bson.M
	require.NoError(t, cursor.All(ctx, &indexSpecs))

	require.Len(t, indexSpecs, 5)

	for _, indexSpec := range indexSpecs {
		if indexSpec["name"] != "_id_" {
			prepareUnique, ok := indexSpec["prepareUnique"].(bool)
			require.True(t, ok)
			require.True(t, prepareUnique)
		}
	}
}

func TestOplogRestoreBypassDocumentValidation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ctx := t.Context()

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	//nolint:errcheck
	defer session.Disconnect(ctx)

	// Prepare the test by creating the necessary collection.
	require.NoError(t, session.Database("mongodump_test_db").Drop(ctx))
	t.Cleanup(func() { _ = session.Database("mongodump_test_db").Drop(t.Context()) })

	oplogFileName := "testdata/oplogs/bson/bypassDocumentValidation.bson"

	for _, bypass := range []bool{false, true} {
		args := []string{
			DirectoryOption, "testdata/coll_without_index",
			OplogReplayOption,
			DropOption,
			OplogFileOption, oplogFileName,
		}

		if bypass {
			args = append(args, BypassDocumentValidationOption)
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		if !bypass {
			require.Error(t, result.Err)
			continue
		}

		require.NoError(t, result.Err)
		count, err := session.Database("mongodump_test_db").
			Collection("coll1").
			CountDocuments(ctx, bson.D{})
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	}
}

func TestOplogRestoreCollModTTLIndex(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testutil.SkipIfFCVLessThan(t, "6.0", "collMod TTL is not supported")

	ctx := t.Context()

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	//nolint:errcheck
	defer session.Disconnect(ctx)

	// Prepare the test by creating the necessary collection.
	require.NoError(t, session.Database("mongodump_test_db").Drop(ctx))
	t.Cleanup(func() { _ = session.Database("mongodump_test_db").Drop(t.Context()) })

	args := []string{
		DirectoryOption, "testdata/collMod_ttl_index",
		OplogReplayOption,
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err)
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err)
	require.Equal(t, int64(0), result.Failures)

	db := session.Database("mongodump_test_db")

	cursor, err := db.RunCommandCursor(ctx, bson.D{
		{"listIndexes", "coll1"},
	})
	require.NoError(t, err)

	var indexSpecs []bson.M
	require.NoError(t, cursor.All(ctx, &indexSpecs))

	require.Len(t, indexSpecs, 2)

	for _, indexSpec := range indexSpecs {
		if indexSpec["name"] != "_id_" {
			expireAfterSeconds, ok := indexSpec["expireAfterSeconds"].(int32)
			require.True(t, ok)
			require.EqualValues(t, 1000, expireAfterSeconds)
		}
	}
}
