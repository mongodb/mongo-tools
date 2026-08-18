// Copyright (C) MongoDB, Inc. 2019-present.
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

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Test files with applyOps transaction entries for
// MongoDB server versions <6.1 and >=6.1, respectively.
const txnTestDataFilePre61 = "testdata/transactions.json"
const txnTestDataFile61Plus = "testdata/transactions-6.1.json"

type txnTestDataMap map[string]*txnTestDataCase

type txnTestDataCase struct {
	Ops       []db.Oplog `bson:"ops"`
	NS        string     `bson:"ns"`
	PostImage []bson.D   `bson:"postimage"`
}

func TestMongorestoreTxns(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	client, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	restore, err := getRestoreWithArgs()
	require.NoError(t, err, "must build a restore instance")

	file := txnTestDataFilePre61
	if restore.serverVersion.GTE(db.Version{6, 1, 0}) {
		file = txnTestDataFile61Plus
	}
	data, err := readTxnTestData(file)
	require.NoError(t, err, "must read the transaction test data")

	// Create test collections (if they don't exist) and clear documents.
	for _, v := range data {
		parts := strings.SplitN(v.NS, ".", 2)
		db := client.Database(parts[0])
		coll := db.Collection(parts[1])
		err := coll.Drop(t.Context())
		require.NoError(t, err, "must drop the existing test collection")
		res := db.RunCommand(t.Context(), bson.D{{"create", parts[1]}})
		require.NoError(t, res.Err(), "must create the test collection")
	}

	// Create a dump directory from transactions.json
	dumpPath := createTxnTestDataDir(t, data)

	args := []string{
		OplogReplayOption,
		DropOption,
		dumpPath,
	}
	restore, err = getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance from the dump path")
	defer restore.Close()

	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")

	for k, v := range data {
		t.Run(k, func(t *testing.T) {
			postImageCheck(t, client, v)
		})
	}
}

// createTxnTestDataDir constructs a dump directory with an oplog.bson
// file that randomly interleaves different cases from the
// testdata/transactions.json file.  This tests that different transactions
// can be cached while continuing processing waiting for a committing entry.
func createTxnTestDataDir(t *testing.T, data txnTestDataMap) string {
	var opStreams [][]db.Oplog
	for _, v := range data {
		if len(v.Ops) != 0 {
			opStreams = append(opStreams, v.Ops)
		}
	}

	dumpDir := testDumpDir{
		dirName: "txntest",
		oplog:   testutil.MergeOplogStreams(opStreams),
	}

	err := dumpDir.Create()
	require.NoError(t, err, "should create the dump directory")

	return dumpDir.Path()
}

func readTxnTestData(filename string) (txnTestDataMap, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't load %s: %v", filename, err)
	}
	var data bson.Raw
	err = bson.UnmarshalExtJSON(b, false, &data)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode JSON: %v", err)
	}
	txnTestData := make(txnTestDataMap)
	err = bson.Unmarshal(data, &txnTestData)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode test data: %v", err)
	}

	return txnTestData, nil
}

func postImageCheck(t *testing.T, client *mongo.Client, c *txnTestDataCase) {
	expected := make(map[int]bson.D)
	for _, v := range c.PostImage {
		id, err := bsonutil.FindIntByKey("_id", &v)
		require.NoError(t, err, "should find the _id of each expected document")
		expected[id] = v
	}

	parts := strings.SplitN(c.NS, ".", 2)
	coll := client.Database(parts[0]).Collection(parts[1])

	cursor, err := coll.Find(t.Context(), bson.D{})
	require.NoError(t, err, "should query the restored collection")
	defer cursor.Close(t.Context())

	var docs []bson.D
	require.NoError(t, cursor.All(t.Context(), &docs), "should read every restored document")

	for _, got := range docs {
		id, err := bsonutil.FindIntByKey("_id", &got)
		require.NoError(t, err, "should find the _id of each restored document")

		want, ok := expected[id]
		require.True(t, ok, "should restore only expected documents, got _id %d", id)

		assert.Equal(t, want, got, "should restore document _id %d unchanged", id)
		delete(expected, id)
	}

	assert.Empty(t, expected, "should restore every expected document")
}
