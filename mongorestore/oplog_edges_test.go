// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestOplogReplayConflict converts oplog_replay_conflict.js: when a dump
// directory already contains an oplog.bson AND --oplogFile points at another
// oplog, mongorestore must fail and apply no data.
func TestOplogReplayConflict(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession()
	require.NoError(t, err)
	coll := client.Database("test").Collection("data")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	result := restoreFromArgs(
		t,
		OplogReplayOption,
		OplogFileOption, "testdata/extra_oplog.bson",
		DirectoryOption, "testdata/dump_oplog_conflict",
	)
	require.ErrorContains(
		t, result.Err, "cannot provide both an oplog.bson file and an oplog file",
		"providing two top-priority oplogs errors",
	)

	count, err := coll.CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err, "counting documents")
	assert.EqualValues(t, 0, count, "no entries are restored when the oplogs conflict")
}

// TestOplogReplayPriorityOplog converts oplog_replay_priority_oplog.js: when a
// dump directory contains a local/oplog.rs.bson AND --oplogFile points at a
// higher-priority oplog, only the priority oplog's entries are applied.
func TestOplogReplayPriorityOplog(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	// This test restores the dump_local_oplog fixture, which contains a
	// local/oplog.rs.bson. Writing local.oplog.rs is only allowed on a
	// standalone. Like the JS test this replaces, it is only meaningful there.
	testutil.SkipUnlessStandalone(t)

	client, err := testutil.GetBareSession()
	require.NoError(t, err)
	testDB := client.Database("test")
	dataColl := testDB.Collection("data")
	opColl := testDB.Collection("op")
	require.NoError(t, dataColl.Drop(context.Background()), "dropping data collection")
	require.NoError(t, opColl.Drop(context.Background()), "dropping op collection")
	t.Cleanup(func() {
		_ = dataColl.Drop(context.Background())
		_ = opColl.Drop(context.Background())
	})
	// On a standalone, applyOps will not auto-create the target namespaces, so
	// they must exist before the oplog is replayed.
	require.NoError(
		t,
		testDB.CreateCollection(context.Background(), "data"),
		"creating data collection",
	)
	require.NoError(
		t,
		testDB.CreateCollection(context.Background(), "op"),
		"creating op collection",
	)

	result := restoreFromArgs(
		t,
		OplogReplayOption,
		OplogFileOption, "testdata/extra_oplog.bson",
		DirectoryOption, "testdata/dump_local_oplog",
	)
	require.NoError(t, result.Err, "restoring with a priority oplog succeeds")

	dataCount, err := dataColl.CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err, "counting data documents")
	assert.EqualValues(
		t, 5, dataCount,
		"all entries from the high-priority --oplogFile are restored",
	)

	opCount, err := opColl.CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err, "counting op documents")
	assert.EqualValues(
		t, 0, opCount,
		"no entries from the low-priority local oplog are restored",
	)
}

// TestOplogReplayNoop converts oplog_replay_noop.js: noop ("n") entries
// interleaved with inserts are skipped, while the inserts are applied.
func TestOplogReplayNoop(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession()
	require.NoError(t, err)
	coll := client.Database("test").Collection("data")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	result := restoreFromArgs(
		t,
		OplogReplayOption,
		DirectoryOption, "testdata/dump_with_noop_in_oplog",
	)
	require.NoError(t, result.Err, "restoring an oplog with noops succeeds")

	count, err := coll.CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err, "counting documents")
	assert.EqualValues(t, 1, count, "the insert after the noops is applied")

	aCount, err := coll.CountDocuments(context.Background(), bson.M{"a": 1})
	require.NoError(t, err, "counting {a:1} documents")
	assert.EqualValues(t, 1, aCount, "the inserted document has the expected contents")
}

// TestOplogReplayPreservesComplexIDOrder converts
// preserve_oplog_structure_order.js: replaying an update op whose _id is a
// multi-field subdocument must preserve the field order, otherwise the server
// rejects the op (the o._id and o2._id must match).
func TestOplogReplayPreservesComplexIDOrder(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)
	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err)

	client, err := testutil.GetBareSession()
	require.NoError(t, err)
	testDB := client.Database("test")
	coll := testDB.Collection("foobar")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })
	require.NoError(
		t, testDB.CreateCollection(context.Background(), "foobar"),
		"creating target collection",
	)

	complexID := bson.D{
		{Key: "a", Value: 1.0},
		{Key: "b", Value: 2.0},
		{Key: "c", Value: 3.0},
		{Key: "d", Value: 5.0},
		{Key: "e", Value: 6.0},
		{Key: "f", Value: 7.0},
		{Key: "g", Value: 8.0},
	}

	// As of SERVER-88158 (8.1+), applyOps no longer upserts by default, so the
	// document the update op targets must already exist.
	if serverVersion.GTE(db.Version{8, 1, 0}) {
		_, err = coll.InsertOne(context.Background(), bson.D{{Key: "_id", Value: complexID}})
		require.NoError(t, err, "pre-inserting the document for the update op")
	}

	result := restoreFromArgs(
		t,
		OplogReplayOption,
		DirectoryOption, "testdata/dump_with_complex_id_oplog",
	)
	require.NoError(
		t, result.Err,
		"replaying an update op with a subdocument _id preserves field order",
	)

	count, err := coll.CountDocuments(context.Background(), bson.D{{Key: "_id", Value: complexID}})
	require.NoError(t, err, "counting documents by the exact subdocument _id")
	assert.EqualValues(t, 1, count, "the document is found by its ordered subdocument _id")
}

// TestOplogReplaySizeSafety converts oplog_replay_size_safety.js with a reduced
// matrix (the JS swept up to a million ops): a large batch of small ops plus a
// batch of ~1MB ops all replay successfully. This guards the batching that
// keeps oplog replay under the 16MB message limit (TOOLS-939).
func TestOplogReplaySizeSafety(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession()
	require.NoError(t, err)
	testDB := client.Database("test")
	coll := testDB.Collection("op")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })
	require.NoError(
		t, testDB.CreateCollection(context.Background(), "op"),
		"creating target collection",
	)

	const (
		smallOps = 50000
		largeOps = 8
		oneMB    = 1024 * 1024
	)

	dir := t.TempDir()
	oplogPath := filepath.Join(dir, "oplog.bson")
	writeOplogInserts(t, oplogPath, "test.op", smallOps, largeOps, oneMB)

	result := restoreFromArgs(t, OplogReplayOption, DirectoryOption, dir)
	require.NoError(t, result.Err, "replaying a large oplog succeeds")

	count, err := coll.CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err, "counting restored documents")
	assert.EqualValues(t, smallOps+largeOps, count, "all oplog entries are inserted")
}

// writeOplogInserts writes an oplog.bson file to path containing smallOps tiny
// insert ops followed by largeOps insert ops each carrying a value of
// largeSize bytes, all targeting namespace ns.
func writeOplogInserts(t *testing.T, path, ns string, smallOps, largeOps, largeSize int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err, "creating oplog fixture file")
	defer func() { require.NoError(t, f.Close(), "closing oplog fixture file") }()

	writeOp := func(id int, value any) {
		op := db.Oplog{
			Version:   2,
			Operation: "i",
			Namespace: ns,
			Object:    bson.D{{Key: "_id", Value: id}, {Key: "x", Value: value}},
		}
		marshaled, err := bson.Marshal(op)
		require.NoError(t, err, "marshaling oplog entry")
		_, err = f.Write(marshaled)
		require.NoError(t, err, "writing oplog entry")
	}

	for i := 0; i < smallOps; i++ {
		writeOp(i, "x")
	}
	big := make([]byte, largeSize)
	for i := range big {
		big[i] = 'x'
	}
	for i := 0; i < largeOps; i++ {
		writeOp(smallOps+i, string(big))
	}
}
