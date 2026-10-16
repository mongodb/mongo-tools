// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package mongorestore

import (
	"fmt"
	"os"
	"testing"

	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const ExistsDB = "restore_collection_exists"

func TestMongoRestoreConnectedToAtlasProxy(t *testing.T) {
	testutil.SkipIfSharded(
		t,
		"it creates and drops a collection in the admin database, which mongos does not allow",
	)
	_, err := testutil.GetBareSession(t)
	if err != nil {
		t.Fatalf("No server available")
	}

	sessionProvider, _, err := testutil.GetBareSessionProvider(t)
	require.NoError(t, err)
	defer sessionProvider.Close()
	restore := &MongoRestore{
		SessionProvider: sessionProvider,
		isAtlasProxy:    true,
		ToolOptions:     &commonOpts.ToolOptions{Namespace: &commonOpts.Namespace{}},
		InputOptions:    &InputOptions{RestoreDBUsersAndRoles: false},
	}
	session, err := restore.SessionProvider.GetSession()
	require.NoError(t, err)

	// This case shouldn't error and should instead not return that it will try to restore users and roles.
	_, err = session.Database("admin").
		Collection("testcol").
		InsertOne(t.Context(), bson.M{})
	require.NoError(t, err)
	require.False(t, restore.ShouldRestoreUsersAndRoles())

	// This case should error because it has explicitly been set to restore users and roles, but thats
	// not possible with an atlas proxy.
	restore.InputOptions.RestoreDBUsersAndRoles = true
	restore.ToolOptions.DB = "test"
	err = restore.ParseAndValidateOptions()
	require.Error(
		t,
		err,
		"cannot restore to the admin database when connected to a MongoDB Atlas free or shared cluster",
	)

	err = session.Database("admin").Collection("testcol").Drop(t.Context())
	require.NoError(t, err)
}

func TestCollectionExists(t *testing.T) {
	_, err := testutil.GetBareSession(t)
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test mongorestore", t, func() {
		sessionProvider, _, err := testutil.GetBareSessionProvider(t)
		So(err, ShouldBeNil)
		defer sessionProvider.Close()

		restore := &MongoRestore{
			SessionProvider: sessionProvider,
		}

		Convey("and some test data in a server", func() {
			session, err := restore.SessionProvider.GetSession()
			So(err, ShouldBeNil)
			_, insertErr := session.Database(ExistsDB).
				Collection("one").
				InsertOne(t.Context(), bson.M{})
			So(insertErr, ShouldBeNil)
			_, insertErr = session.Database(ExistsDB).
				Collection("two").
				InsertOne(t.Context(), bson.M{})
			So(insertErr, ShouldBeNil)
			_, insertErr = session.Database(ExistsDB).
				Collection("three").
				InsertOne(t.Context(), bson.M{})
			So(insertErr, ShouldBeNil)

			Convey("collections that exist should return true", func() {
				exists, err := restore.CollectionExists(ExistsDB, "one")
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)
				exists, err = restore.CollectionExists(ExistsDB, "two")
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)
				exists, err = restore.CollectionExists(ExistsDB, "three")
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)

				Convey("and those that do not exist should return false", func() {
					exists, err = restore.CollectionExists(ExistsDB, "four")
					So(err, ShouldBeNil)
					So(exists, ShouldBeFalse)
				})
			})

			Reset(func() {
				err = session.Database(ExistsDB).Drop(t.Context())
				So(err, ShouldBeNil)
			})
		})

		Convey("and a fake cache should be used instead of the server when it exists", func() {
			restore.knownCollections = map[string][]string{
				ExistsDB: {"cats", "dogs", "snakes"},
			}
			exists, err := restore.CollectionExists(ExistsDB, "dogs")
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)
			exists, err = restore.CollectionExists(ExistsDB, "two")
			So(err, ShouldBeNil)
			So(exists, ShouldBeFalse)
		})
	})
}

const indexCollationTestDataFile = "testdata/index_collation.json"

func TestIndexGetsSimpleCollation(t *testing.T) {
	metadata, err := readCollationTestData(indexCollationTestDataFile)
	if err != nil {
		t.Fatalf("Error reading data file: %v", err)
	}

	dumpDir := testDumpDir{
		dirName: "index_collation",
		collections: []testCollData{{
			ns:       "test.foo",
			metadata: metadata,
		}},
	}

	err = dumpDir.Create()
	if err != nil {
		t.Fatalf("Error reading data file: %v", err)
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			DropOption,
			dumpDir.Path(),
		}
		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		result := restore.Restore()
		So(result.Err, ShouldBeNil)
	})
}

func readCollationTestData(filename string) (bson.D, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't load %s: %v", filename, err)
	}
	var data bson.D
	err = bson.UnmarshalExtJSON(b, false, &data)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode JSON: %v", err)
	}
	return data, nil
}
