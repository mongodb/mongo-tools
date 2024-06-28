// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/failpoint"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestErrorOnImportCollection(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("An importCollection oplog entry should error", t, func() {
		rawOp, err := ioutil.ReadFile("./testdata/importCollection.bson")
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

	log.SetWriter(ioutil.Discard)

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

	ctx := context.Background()

	md := simpleMongoDumpInstance()
	md.ToolOptions.Namespace.DB = ""
	md.OutputOptions.Oplog = true
	md.OutputOptions.Out = "vectored_inserts"

	require.NoError(t, md.Init())

	// Enable a failpoint so that the test can create oplogs during dump.
	failpoint.ParseFailpoints("PauseBeforeDumping")
	defer failpoint.Reset()

	require.NoError(t, vectoredInsert(ctx))
	defer tearDownMongoDumpTestData()

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
	defer oplogFile.Close()

	contents, err := ioutil.ReadAll(oplogFile)
	var oplog bson.D
	require.NoError(t, bson.Unmarshal(contents, &oplog))

	require.Equal(t, int32(1), oplog.Map()["multiOpType"])
}

func vectoredInsert(ctx context.Context) error {
	client, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	f := false
	if sessionErr := client.UseSessionWithOptions(
		ctx,
		&options.SessionOptions{CausalConsistency: &f},
		func(sessionContext mongo.SessionContext) error {
			docs := []interface{}{
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
