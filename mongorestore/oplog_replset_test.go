// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestOplogReplayFromLocalOplogRS converts oplog_replay_local_rs.js and
// dumprestore7.js into a single replica-set test: it dumps local.oplog.rs with
// a $gt timestamp query (capturing only the ops written after a checkpoint),
// then replays that dumped oplog with --oplogReplay --oplogFile and verifies
// exactly those ops are applied.
func TestOplogReplayFromLocalOplogRS(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	// local.oplog.rs only exists on a replica set.
	testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)

	const (
		dbName   = "testdr_oplogrs"
		collName = "coll"
	)

	client, err := testutil.GetBareSession()
	require.NoError(t, err)

	ctx := context.Background()
	testDB := client.Database(dbName)
	coll := testDB.Collection(collName)
	ns := dbName + "." + collName
	require.NoError(t, testDB.Drop(ctx), "dropping test db")
	t.Cleanup(func() { _ = testDB.Drop(context.Background()) })

	// First batch: these ops precede the checkpoint and must NOT be replayed.
	for i := 0; i < 5; i++ {
		_, err = coll.InsertOne(ctx, bson.D{{Key: "_id", Value: i}, {Key: "batch", Value: 1}})
		require.NoError(t, err, "inserting first batch")
	}

	checkpoint := latestOplogTimestamp(t, client)

	// Second batch: these ops follow the checkpoint and must be replayed.
	const secondBatch = 7
	for i := 0; i < secondBatch; i++ {
		_, err = coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 100 + i}, {Key: "batch", Value: 2}})
		require.NoError(t, err, "inserting second batch")
	}

	dumpDir := t.TempDir()
	query := fmt.Sprintf(
		`{"ts":{"$gt":{"$timestamp":{"t":%d,"i":%d}}},"ns":%q,"op":"i"}`,
		checkpoint.T, checkpoint.I, ns,
	)
	require.NoError(t, runDump(t, baseToolOpts(t), dumpDir, func(md *mongodump.MongoDump) {
		md.ToolOptions.Namespace = &options.Namespace{DB: "local", Collection: "oplog.rs"}
		md.InputOptions.Query = query
	}), "dumping local.oplog.rs with a timestamp query")

	require.NoError(t, coll.Drop(ctx), "dropping collection before replay")
	require.NoError(
		t,
		testDB.CreateCollection(ctx, collName),
		"recreating collection before replay",
	)

	oplogFile := filepath.Join(dumpDir, "local", "oplog.rs.bson")
	result := restoreFromArgs(
		t,
		OplogReplayOption,
		OplogFileOption, oplogFile,
		DirectoryOption, t.TempDir(),
	)
	require.NoError(t, result.Err, "replaying the dumped oplog succeeds")

	count, err := coll.CountDocuments(ctx, bson.M{})
	require.NoError(t, err, "counting replayed documents")
	assert.EqualValues(
		t, secondBatch, count,
		"only the ops after the checkpoint are replayed",
	)

	batch1Count, err := coll.CountDocuments(ctx, bson.M{"batch": 1})
	require.NoError(t, err, "counting first-batch documents")
	assert.EqualValues(t, 0, batch1Count, "ops before the checkpoint are not replayed")
}

// latestOplogTimestamp returns the ts of the most recent entry in
// local.oplog.rs.
func latestOplogTimestamp(t *testing.T, client *mongo.Client) bson.Timestamp {
	t.Helper()
	var entry struct {
		TS bson.Timestamp `bson:"ts"`
	}
	err := client.Database("local").Collection("oplog.rs").
		FindOne(
			context.Background(),
			bson.D{},
			mopts.FindOne().SetSort(bson.D{{Key: "$natural", Value: -1}}),
		).
		Decode(&entry)
	require.NoError(t, err, "reading latest oplog entry")
	return entry.TS
}
