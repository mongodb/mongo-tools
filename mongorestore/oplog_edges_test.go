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

// TestOplogReplayConflict checks that when a dump directory already contains an
// oplog.bson and --oplogFile points at another oplog, mongorestore fails and
// applies no data rather than silently choosing one of the two.
func TestOplogReplayConflict(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession(t)
	require.NoError(t, err)
	coll := client.Database("test").Collection("data")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	result := runRestoreFromArgs(
		t,
		OplogReplayOption,
		OplogFileOption, "testdata/extra_oplog.bson",
		DirectoryOption, "testdata/dump_oplog_conflict",
	)
	require.ErrorContains(
		t, result.Err, "cannot provide both an oplog.bson file and an oplog file",
		"providing two top-priority oplogs errors",
	)

	count, err := coll.CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err, "counting documents")
	assert.EqualValues(t, 0, count, "no entries are restored when the oplogs conflict")
}

// TestOplogReplayPriorityOplog checks that when a dump directory contains a
// local/oplog.rs.bson and --oplogFile points at a higher-priority oplog, only
// the priority oplog's entries are applied.
func TestOplogReplayPriorityOplog(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	// This test restores the dump_local_oplog fixture, which contains a
	// local/oplog.rs.bson. Writing local.oplog.rs as a collection is only
	// permitted on a standalone: a replica set rejects direct oplog writes, and
	// mongos rejects writes to the local database.
	testutil.SkipUnlessStandalone(t)

	client, err := testutil.GetBareSession(t)
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

	result := runRestoreFromArgs(
		t,
		OplogReplayOption,
		OplogFileOption, "testdata/extra_oplog.bson",
		DirectoryOption, "testdata/dump_local_oplog",
	)
	require.NoError(t, result.Err, "restoring with a priority oplog succeeds")

	dataCount, err := dataColl.CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err, "counting data documents")
	assert.EqualValues(
		t, 5, dataCount,
		"all entries from the high-priority --oplogFile are restored",
	)

	opCount, err := opColl.CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err, "counting op documents")
	assert.EqualValues(
		t, 0, opCount,
		"no entries from the low-priority local oplog are restored",
	)
}

// TestOplogReplayNoop checks that noop ("n") entries interleaved with inserts
// are skipped while the inserts around them are still applied.
func TestOplogReplayNoop(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession(t)
	require.NoError(t, err)
	coll := client.Database("test").Collection("data")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	result := runRestoreFromArgs(
		t,
		OplogReplayOption,
		DirectoryOption, "testdata/dump_with_noop_in_oplog",
	)
	require.NoError(t, result.Err, "restoring an oplog with noops succeeds")

	count, err := coll.CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err, "counting documents")
	assert.EqualValues(t, 1, count, "the insert after the noops is applied")

	aCount, err := coll.CountDocuments(context.Background(), bson.D{{"a", 1}})
	require.NoError(t, err, "counting {a:1} documents")
	assert.EqualValues(t, 1, aCount, "the inserted document has the expected contents")
}

// TestOplogReplayPreservesComplexIDOrder checks that replaying an update op
// whose _id is a multi-field subdocument preserves that subdocument's field
// order. The server matches o._id against o2._id exactly, so a reordered _id
// applies the op to nothing.
//
// The oplog is written here rather than read from a checked-in fixture so the
// field order is visible, and so it can be a deliberately unsorted one.
//
// The replay is repeated because the input bytes are identical every time, so
// any variation comes from mongorestore itself. If it were to carry the _id
// through a Go map, the field order would come out as one of only about as many
// orders as there are fields, so a single attempt could land on the right one by
// chance.
func TestOplogReplayPreservesComplexIDOrder(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	sessionProvider, _, err := testutil.GetBareSessionProvider(t)
	require.NoError(t, err)
	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err)

	client, err := testutil.GetBareSession(t)
	require.NoError(t, err)
	testDB := client.Database("test")
	coll := testDB.Collection("foobar")
	require.NoError(t, coll.Drop(context.Background()), "dropping target collection")
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })
	require.NoError(
		t, testDB.CreateCollection(context.Background(), "foobar"),
		"creating target collection",
	)

	complexID := complexOplogID()
	// As of SERVER-88158 (8.1+), applyOps no longer upserts by default, so the
	// document the update op targets must already exist.
	needsTarget := serverVersion.GTE(db.Version{8, 1, 0})

	dumpDir := t.TempDir()
	writeOplogUpdate(t, filepath.Join(dumpDir, "oplog.bson"), "test.foobar", complexID)

	const complexIDReplayAttempts = 10
	for attempt := range complexIDReplayAttempts {
		_, err = coll.DeleteMany(context.Background(), bson.D{})
		require.NoError(t, err, "clearing the collection before attempt %d", attempt)

		if needsTarget {
			_, err = coll.InsertOne(context.Background(), bson.D{{"_id", complexID}})
			require.NoError(t, err, "pre-inserting the target document")
		}

		result := runRestoreFromArgs(
			t,
			OplogReplayOption,
			DirectoryOption, dumpDir,
		)
		require.NoError(t, result.Err, "replaying the update op on attempt %d", attempt)

		// The update sets "foo", so finding it proves the op matched rather than
		// silently applying to nothing. Counting the document alone would not:
		// where the target is pre-inserted, it is there either way.
		var updated struct {
			Foo string `bson:"foo"`
		}
		err = coll.FindOne(context.Background(), bson.D{{"_id", complexID}}).Decode(&updated)
		require.NoError(
			t, err,
			"finding the document by its ordered subdocument _id on attempt %d",
			attempt,
		)
		assert.Equal(
			t, "bar", updated.Foo,
			"the replayed update matched the document on attempt %d",
			attempt,
		)
	}
}

// complexOplogID returns the subdocument used as an _id by the field-order test.
// The fields are deliberately not in alphabetical order: sorting them is the most
// likely way for the order to be lost, and a sorted _id would be
// indistinguishable from a correct one if the fields were named in order.
func complexOplogID() bson.D {
	return bson.D{
		{"g", 8.0},
		{"c", 3.0},
		{"a", 1.0},
		{"f", 7.0},
		{"b", 2.0},
		{"e", 6.0},
		{"d", 5.0},
	}
}

// writeOplogUpdate writes a one-entry oplog holding an update op whose _id is a
// subdocument. mongorestore has to reproduce that subdocument's field order in
// both o and o2, because the server compares them exactly.
func writeOplogUpdate(t *testing.T, path, ns string, id bson.D) {
	t.Helper()

	op := db.Oplog{
		Timestamp: bson.Timestamp{T: 1439225650, I: 1},
		Version:   2,
		Operation: "u",
		Namespace: ns,
		Query:     bson.D{{"_id", id}},
		Object:    bson.D{{"_id", id}, {"foo", "bar"}},
	}

	marshaled, err := bson.Marshal(op)
	require.NoError(t, err, "marshaling the update oplog entry")
	require.NoError(t, os.WriteFile(path, marshaled, 0644), "writing the oplog fixture")
}

// TestOplogReplaySizeSafety replays a large batch of small ops together with a
// batch of roughly 1MB ops, guarding the batching that keeps oplog replay
// within the server's message size limit (TOOLS-939).
func TestOplogReplaySizeSafety(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client, err := testutil.GetBareSession(t)
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

	result := runRestoreFromArgs(t, OplogReplayOption, DirectoryOption, dir)
	require.NoError(t, result.Err, "replaying a large oplog succeeds")

	count, err := coll.CountDocuments(context.Background(), bson.D{})
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
			Object:    bson.D{{"_id", id}, {"x", value}},
		}
		marshaled, err := bson.Marshal(op)
		require.NoError(t, err, "marshaling oplog entry")
		_, err = f.Write(marshaled)
		require.NoError(t, err, "writing oplog entry")
	}

	for i := range smallOps {
		writeOp(i, "x")
	}
	big := make([]byte, largeSize)
	for i := range big {
		big[i] = 'x'
	}
	for i := range largeOps {
		writeOp(smallOps+i, string(big))
	}
}
