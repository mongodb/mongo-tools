// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/failpoint"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestErrorOnImportCollection(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("An importCollection oplog entry should error", t, func() {
		rawOp, err := os.ReadFile("./testdata/importCollection.bson")
		So(err, ShouldBeNil)

		err = oplogDocumentValidator(rawOp)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "cannot dump with oplog while importCollection occurs")
	})
}

// TestOplogDumpVectoredInsertsOplog tests dumping oplogs that are from vectored inserts.
// They have a special oplog format.
func TestOplogDumpVectoredInsertsOplog(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	// Oplog is not available in a standalone topology.
	testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)

	log.SetWriter(io.Discard)

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "8.0"); err != nil || cmp < 0 {
		if err != nil {
			t.Errorf("error getting FCV: %v", err)
		}
		t.Skipf("Requires server with FCV 8.0 or later; found %v", fcv)
	}

	ctx := t.Context()

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	md.ToolOptions.DB = ""
	md.OutputOptions.Oplog = true
	md.OutputOptions.Out = "vectored_inserts"

	require.NoError(t, md.Init())

	// Enable a failpoint so that the test can create oplogs during dump.
	failpoint.ParseFailpoints("PauseBeforeDumping")
	defer failpoint.Reset()

	require.NoError(t, vectoredInsert(ctx))
	//nolint:errcheck
	defer tearDownMongoDumpTestData(t)

	require.NoError(t, md.Dump())

	path, err := os.Getwd()
	require.NoError(t, err)

	dumpDir := util.ToUniversalPath(filepath.Join(path, "vectored_inserts"))
	dumpDBDir := util.ToUniversalPath(filepath.Join(dumpDir, testDB))
	oplogFilePath := util.ToUniversalPath(filepath.Join(dumpDir, "oplog.bson"))
	require.True(t, fileDirExists(dumpDir))
	require.True(t, fileDirExists(dumpDBDir))
	require.True(t, fileDirExists(oplogFilePath))

	defer os.RemoveAll(dumpDir)

	oplogFile, err := os.Open(oplogFilePath)
	require.NoError(t, err)
	defer oplogFile.Close()

	contents, err := io.ReadAll(oplogFile)
	require.NoError(t, err)

	var oplog bson.D
	require.NoError(t, bson.Unmarshal(contents, &oplog))

	require.Equal(t, int32(1), bsonutil.ToMap(oplog)["multiOpType"])
}

func vectoredInsert(ctx context.Context) error {
	client, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	if sessionErr := client.UseSessionWithOptions(
		ctx,
		options.Session().SetCausalConsistency(false),
		func(sessionContext context.Context) error {
			docs := []any{
				bson.D{{"_id", 100}, {"a", 1}},
				bson.D{{"_id", 200}, {"a", 2}},
			}
			_, insertErr := client.Database(testDB).Collection(testCollectionNames[0]).InsertMany(ctx, docs)
			if insertErr != nil {
				return insertErr
			}

			return nil
		},
	); sessionErr != nil {
		return sessionErr
	}

	return nil
}

func TestOplogDumpCollModPrepareUnique(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	// Oplog is not available in a standalone topology.
	testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)

	ctx := t.Context()

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "6.0"); err != nil || cmp < 0 {
		if err != nil {
			t.Errorf("error getting FCV: %v", err)
		}
		t.Skipf("Requires server with FCV 6.0 or later; found %v", fcv)
	}

	testCollName := testCollectionNames[0]

	err = session.Database(testDB).CreateCollection(ctx, testCollName)
	require.NoError(t, err)
	//nolint:errcheck
	defer session.Database(testDB).Collection(testCollName).Drop(ctx)

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	md.ToolOptions.DB = ""
	md.OutputOptions.Oplog = true
	md.OutputOptions.Out = "collMod_prepareUnique"

	require.NoError(t, md.Init())

	// Enable a failpoint so that the test can create oplogs during dump.
	failpoint.ParseFailpoints(failpoint.PauseBeforeDumping)
	defer failpoint.Reset()

	go func() {
		require.NoError(t, createIndexesAndRunCollModPrepareUnique(ctx))
	}()

	//nolint:errcheck
	defer tearDownMongoDumpTestData(t)

	require.NoError(t, md.Dump())

	path, err := os.Getwd()
	require.NoError(t, err)

	dumpDir := util.ToUniversalPath(filepath.Join(path, "collMod_prepareUnique"))
	dumpDBDir := util.ToUniversalPath(filepath.Join(dumpDir, testDB))
	oplogFilePath := util.ToUniversalPath(filepath.Join(dumpDir, "oplog.bson"))
	require.True(t, fileDirExists(dumpDir))
	require.True(t, fileDirExists(dumpDBDir))
	require.True(t, fileDirExists(oplogFilePath))

	defer os.RemoveAll(dumpDir)

	oplogFile, err := os.Open(oplogFilePath)
	require.NoError(t, err)
	defer oplogFile.Close()

	bsonSrc := db.NewDecodedBSONSource(db.NewBufferlessBSONSource(oplogFile))
	prepareUniqueTrueCount := 0
	prepareUniqueFalseCount := 0

	var oplog db.Oplog
	for bsonSrc.Next(&oplog) {
		require.NoError(t, bsonSrc.Err())

		if oplog.Namespace == testDB+".$cmd" {
			indexDoc, ok := bsonutil.ToMap(oplog.Object)["index"].(bson.D)
			if ok {
				if bsonutil.ToMap(indexDoc)["prepareUnique"] == true {
					prepareUniqueTrueCount++
				} else {
					prepareUniqueFalseCount++
				}
			}
		}
	}
	require.NoError(t, oplogFile.Close())
	require.Equal(t, 8, prepareUniqueTrueCount)
	require.Equal(t, 4, prepareUniqueFalseCount)
}

func createIndexesAndRunCollModPrepareUnique(ctx context.Context) error {
	client, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	testCollName := testCollectionNames[0]

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{"a", 1}},
		},
		{
			Keys:    bson.D{{"b", 1}},
			Options: options.Index().SetHidden(true),
		},
		{
			Keys:    bson.D{{"c", 1}},
			Options: options.Index().SetExpireAfterSeconds(100000),
		},
		{
			Keys:    bson.D{{"d", 1}},
			Options: options.Index().SetExpireAfterSeconds(100000).SetHidden(true),
		},
	}

	_, err = client.Database(testDB).Collection(testCollName).Indexes().CreateMany(
		ctx,
		indexes,
	)
	if err != nil {
		return err
	}

	for _, index := range indexes {
		for _, prepareUnique := range []bool{true, false, true} {
			res := client.Database(testDB).RunCommand(
				ctx,
				bson.D{
					{"collMod", testCollName},
					{"index", bson.D{
						{"keyPattern", index.Keys},
						{"prepareUnique", prepareUnique},
					}},
				},
			)
			if res.Err() != nil {
				return res.Err()
			}
		}
	}

	return nil
}
