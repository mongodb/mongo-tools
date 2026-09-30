// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"context"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestOplogReplayLimit restores one dump three ways -- without --oplogReplay,
// with it, and with it plus an --oplogLimit that falls between two of the oplog
// entries -- and checks which documents each way produces. Together they pin
// down what --oplogLimit does: entries at or after the limit are not applied,
// and everything before it still is.
func TestOplogReplayLimit(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession(t)
	require.NoError(t, err, "getting a session")

	coll := client.Database("test").Collection("data")
	// Not t.Context(): that context is already canceled by the time cleanup runs.
	t.Cleanup(func() {
		require.NoError(
			t,
			coll.Drop(context.Background()),
			"dropping the target collection when the test is done, so it leaves nothing behind",
		)
	})

	dumpPath := createOplogLimitDump(t)

	cases := []struct {
		name    string
		args    []string
		wantIDs []int
	}{
		{
			name:    "without --oplogReplay only the collection's documents are restored",
			args:    []string{DirectoryOption, dumpPath},
			wantIDs: intsThrough(9),
		},
		{
			name:    "with --oplogReplay the oplog's inserts are applied too",
			args:    []string{OplogReplayOption, DirectoryOption, dumpPath},
			wantIDs: intsThrough(14),
		},
		{
			// The limit sits between the fourth oplog entry and the fifth, which is
			// the one dated decades later.
			name: "--oplogLimit excludes the entries at or after it",
			args: []string{
				OplogReplayOption,
				OplogLimitOption, "1416342266:0",
				DirectoryOption, dumpPath,
			},
			wantIDs: intsThrough(13),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(
				t,
				coll.Drop(t.Context()),
				"dropping the target collection before restoring into it",
			)

			result := runRestoreFromArgs(t, c.args...)
			require.NoError(t, result.Err, "the restore succeeds")

			assert.Equal(
				t,
				c.wantIDs,
				restoredDocumentIDs(t, coll),
				"exactly these documents are restored",
			)
		})
	}
}

// createOplogLimitDump builds a dump of test.data holding the documents whose
// _ids are 0 through 9, alongside an oplog.bson inserting 10 through 14. Four of
// the oplog timestamps are an instant apart and the fifth is decades later, so
// that a limit can go between them without any dependence on when the test runs.
//
// Coverage note: the dump carries no collection metadata, so the restore of
// collection options and indexes is not exercised here. metadata_test.go covers
// that path.
func createOplogLimitDump(t *testing.T) string {
	const ns = "test.data"

	docs := make([]bson.D, 0, 10)
	for _, id := range intsThrough(9) {
		docs = append(docs, bson.D{{"_id", id}})
	}

	timestamps := []bson.Timestamp{
		{T: 1416342265, I: 2},
		{T: 1416342265, I: 3},
		{T: 1416342265, I: 4},
		{T: 1416342265, I: 5},
		{T: 1500000000, I: 1},
	}
	oplog := make([]db.Oplog, 0, len(timestamps))
	for i, ts := range timestamps {
		oplog = append(oplog, db.Oplog{
			Timestamp: ts,
			Version:   2,
			Operation: "i",
			Namespace: ns,
			Object:    bson.D{{"_id", 10 + i}},
		})
	}

	dumpDir := testDumpDir{
		dirName:     "dump_with_oplog_limit",
		oplog:       oplog,
		collections: []testCollData{{ns: ns, docs: docs}},
	}
	require.NoError(t, dumpDir.Create(), "creating the dump directory")
	t.Cleanup(func() {
		require.NoError(t, dumpDir.Cleanup(), "removing the dump directory")
	})

	return dumpDir.Path()
}

// restoredDocumentIDs returns the collection's _ids in ascending order, so that
// a comparison against them reports both what is missing and what should not be
// there.
func restoredDocumentIDs(t *testing.T, coll *mongo.Collection) []int {
	cursor, err := coll.Find(t.Context(), bson.D{}, options.Find().SetSort(bson.D{{"_id", 1}}))
	require.NoError(t, err, "finding the restored documents")

	var docs []struct {
		ID int `bson:"_id"`
	}
	require.NoError(t, cursor.All(t.Context(), &docs), "decoding the restored documents")

	ids := make([]int, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}

	return ids
}

func intsThrough(last int) []int {
	ints := make([]int, 0, last+1)
	for i := range last + 1 {
		ints = append(ints, i)
	}

	return ints
}
