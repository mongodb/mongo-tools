// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"context"
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	. "github.com/smartystreets/goconvey/convey"

	"io"
	"io/ioutil"
	"os"
	"testing"
)

func init() {
	// bump up the verbosity to make checking debug log output possible
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

var (
	testArchive          = "testdata/test.bar.archive"
	testArchiveWithOplog = "testdata/dump-w-oplog.archive"
	testBadFormatArchive = "testdata/bad-format.archive"
)

func TestMongorestoreShortArchive(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			ArchiveOption + "=" + testArchive,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			DropOption,
		}

		file, err := os.Open(testArchive)
		So(file, ShouldNotBeNil)
		So(err, ShouldBeNil)

		fi, err := file.Stat()
		So(fi, ShouldNotBeNil)
		So(err, ShouldBeNil)

		fileSize := fi.Size()

		for i := fileSize; i >= 0; i -= fileSize / 10 {
			log.Logvf(log.Always, "Restoring from the first %v bytes of a archive of size %v", i, fileSize)

			_, err = file.Seek(0, 0)
			So(err, ShouldBeNil)

			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)
			defer restore.Close()

			restore.archive = &archive.Reader{
				Prelude: &archive.Prelude{},
				In:      ioutil.NopCloser(io.LimitReader(file, i)),
			}

			result := restore.Restore()
			if i == fileSize {
				So(result.Err, ShouldBeNil)
			} else {
				So(result.Err, ShouldNotBeNil)
			}
		}
	})
}

func TestMongorestoreArchiveWithOplog(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			ArchiveOption + "=" + testArchiveWithOplog,
			OplogReplayOption,
			DropOption,
		}
		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		result := restore.Restore()
		So(result.Err, ShouldBeNil)
		So(result.Failures, ShouldEqual, 0)
		So(result.Successes, ShouldNotEqual, 0)
	})
}

func TestMongorestoreBadFormatArchive(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			ArchiveOption + "=" + testBadFormatArchive,
			DropOption,
		}
		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		result := restore.Restore()
		Convey("A mongorestore on an archive with a bad format should error out instead of hang", func() {
			So(result.Err, ShouldNotBeNil)
			So(result.Failures, ShouldEqual, 0)
			So(result.Successes, ShouldEqual, 0)
		})
	})
}

// ----------------------------------------------------------------------
// All tests from this point onwards use testify, not convey. See the
// CONTRIBUING.md file in the top level of the repo for details on how to
// write tests using testify.
// ----------------------------------------------------------------------

func TestMongorestoreArchiveAdminNamespaces(t *testing.T) {
	require := require.New(t)

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	// TODO (Johnny) what is the correct FCV to use?
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.0"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 4.0 or later and we have %s", fcv)
	}

	t.Run("restore from archive", func(t *testing.T) {
		testRestoreAdminNamespaces(t)
	})

	t.Run("restore from archive as atlas proxy", func(t *testing.T) {
		testRestoreAdminNamespacesAsAtlasProxy(t)
	})

}

func testRestoreAdminNamespaces(t *testing.T) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	adminDB := session.Database("admin")
	defer func() {
		err = testDB.Drop(nil)
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	collections := []*mongo.Collection{
		createCollectionWithTestDocument(t, testDB, "admincollection"),
		createCollectionWithTestDocument(t, adminDB, "admincollection"),
		createCollectionWithTestDocument(t, testDB, "collection"),
		createCollectionWithTestDocument(t, adminDB, "collection"),
	}

	withArchiveMongodump(t, func(file string) {

		for _, collection := range collections {
			err := collection.Drop(context.Background())
			require.NoError(err, "can drop collection")
		}

		restore, err := getRestoreWithArgs(
			DropOption,
			ArchiveOption+"="+file,
		)
		require.NoError(err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(result.Err, "can run mongorestore")
		require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		for _, collection := range collections {
			requireCollectionHasNumDocuments(t, collection, 1)
		}

	})
}

func testRestoreAdminNamespacesAsAtlasProxy(t *testing.T) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	adminDB := session.Database("admin")
	defer func() {
		err = testDB.Drop(nil)
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	testDBAdminCollection := createCollectionWithTestDocument(t, testDB, "admincollection")
	adminDBAdminCollection := createCollectionWithTestDocument(t, adminDB, "admincollection")
	testDBCollection := createCollectionWithTestDocument(t, testDB, "collection")
	adminDBCollection := createCollectionWithTestDocument(t, adminDB, "collection")

	withArchiveMongodump(t, func(file string) {

		for _, collection := range []*mongo.Collection{
			testDBAdminCollection,
			adminDBAdminCollection,
			testDBCollection,
			adminDBCollection,
		} {
			err := collection.Drop(context.Background())
			require.NoError(err, "can drop collection")
		}

		restore, err := getAtlasProxyRestoreWithArgs(
			DropOption,
			ArchiveOption+"="+file,
		)
		require.NoError(err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(result.Err, "can run mongorestore")
		require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		requireCollectionHasNumDocuments(t, testDBAdminCollection, 1)
		requireCollectionHasNumDocuments(t, adminDBAdminCollection, 0)
		requireCollectionHasNumDocuments(t, testDBCollection, 1)
		requireCollectionHasNumDocuments(t, adminDBCollection, 0)
	})
}

func requireCollectionHasNumDocuments(t *testing.T, collection *mongo.Collection, numDocuments int64) {
	require := require.New(t)
	count, err := collection.CountDocuments(context.Background(), bson.M{})
	require.NoError(err, "can count documents")
	require.EqualValues(0, count, "found %d document(s)", numDocuments)
}

func createCollectionWithTestDocument(t *testing.T, db *mongo.Database, collectionName string) *mongo.Collection {
	require := require.New(t)
	collection := db.Collection(collectionName)
	_, err := collection.InsertOne(
		context.Background(),
		bson.M{"key": "value"},
	)
	require.NoError(err, "can insert documents into collection")
	return collection
}

func getAtlasProxyRestoreWithArgs(args ...string) (*MongoRestore, error) {
	restore, err := getRestoreWithArgs(args...)
	if err != nil {
		return nil, err
	}
	restore.isAtlasProxy = true
	return restore, nil
}

func withArchiveMongodump(t *testing.T, testCase func(string)) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	file := filepath.Join(dir, "archive")
	runArchiveMongodump(t, file)
	testCase(file)
}

func runArchiveMongodump(t *testing.T, file string) string {
	require := require.New(t)
	runMongodumpWithArgs(t, ArchiveOption+"="+file)
	_, err := os.Stat(file)
	require.NoError(err, "dump created archive data file")
	return file
}
