// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/intents"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const ExistsDB = "restore_collection_exists"

func TestMongoRestoreConnectedToAtlasProxy(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	sessionProvider, _, err := testutil.GetBareSessionProvider()
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
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	t.Run("collections that exist return true and others return false", func(t *testing.T) {
		restore := newCollectionExistsRestore(t)

		session, err := restore.SessionProvider.GetSession()
		require.NoError(t, err, "should get a session")
		t.Cleanup(func() {
			// t.Context() is already canceled by the time cleanups run.
			assert.NoError(
				t,
				session.Database(ExistsDB).Drop(context.Background()),
				"should drop the test database",
			)
		})

		_, insertErr := session.Database(ExistsDB).
			Collection("one").
			InsertOne(t.Context(), bson.M{})
		require.NoError(t, insertErr, "should insert into collection one")
		_, insertErr = session.Database(ExistsDB).
			Collection("two").
			InsertOne(t.Context(), bson.M{})
		require.NoError(t, insertErr, "should insert into collection two")
		_, insertErr = session.Database(ExistsDB).
			Collection("three").
			InsertOne(t.Context(), bson.M{})
		require.NoError(t, insertErr, "should insert into collection three")

		exists, err := restore.CollectionExists(ExistsDB, "one")
		require.NoError(t, err, "should check collection one")
		assert.True(t, exists, "collection one should exist")
		exists, err = restore.CollectionExists(ExistsDB, "two")
		require.NoError(t, err, "should check collection two")
		assert.True(t, exists, "collection two should exist")
		exists, err = restore.CollectionExists(ExistsDB, "three")
		require.NoError(t, err, "should check collection three")
		assert.True(t, exists, "collection three should exist")

		exists, err = restore.CollectionExists(ExistsDB, "four")
		require.NoError(t, err, "should check a collection that was never created")
		assert.False(t, exists, "collection four should not exist")
	})

	t.Run("a fake cache is used instead of the server when it exists", func(t *testing.T) {
		restore := newCollectionExistsRestore(t)
		restore.knownCollections = map[string][]string{
			ExistsDB: {"cats", "dogs", "snakes"},
		}
		exists, err := restore.CollectionExists(ExistsDB, "dogs")
		require.NoError(t, err, "should check a known collection")
		assert.True(t, exists, "dogs should be reported present from the known collections cache")
		exists, err = restore.CollectionExists(ExistsDB, "two")
		require.NoError(t, err, "should check a collection not in the cache")
		assert.False(
			t,
			exists,
			"two should not be reported present since it is not in the known collections cache",
		)
	})
}

// newCollectionExistsRestore returns a MongoRestore with a fresh session
// provider and registers a cleanup that closes it, mirroring the original's
// defer sessionProvider.Close(). Dropping ExistsDB is the caller's
// responsibility: only the subtest that inserts data needs to register that
// cleanup, matching the scope of the GoConvey Reset this replaces, which
// only ran on the "and some test data in a server" leaf.
func newCollectionExistsRestore(t *testing.T) *MongoRestore {
	t.Helper()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "should get a session provider")
	t.Cleanup(sessionProvider.Close)

	return &MongoRestore{SessionProvider: sessionProvider}
}

func TestGetDumpAuthVersion(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	version := db.Version{8, 0, 0}

	t.Run("and no --restoreDbUsersAndRoles", func(t *testing.T) {
		cases := []struct {
			name            string
			authVersionFile string
			expectedVersion int
		}{
			{"auth version 1 should be detected", "", 1},
			{"auth version 3 should be detected", "testdata/auth_version_3.bson", 3},
			{"auth version 5 should be detected", "testdata/auth_version_5.bson", 5},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				restore := &MongoRestore{
					InputOptions: &InputOptions{},
					ToolOptions:  &commonOpts.ToolOptions{},
					NSOptions:    &NSOptions{},
					manager:      intents.NewIntentManager(),
				}
				if tc.authVersionFile != "" {
					putAuthVersionIntent(restore, version, tc.authVersionFile)
				}

				got, err := restore.GetDumpAuthVersion()
				require.NoError(t, err, "should determine the auth version")
				assert.Equal(
					t,
					tc.expectedVersion,
					got,
					"should detect auth version %d",
					tc.expectedVersion,
				)
			})
		}
	})

	t.Run("using --restoreDbUsersAndRoles", func(t *testing.T) {
		cases := []struct {
			name            string
			authVersionFile string
			expectedVersion int
		}{
			{"auth version 3 should be detected when no file exists", "", 3},
			{
				"auth version 3 should be detected when a version 3 file exists",
				"testdata/auth_version_3.bson",
				3,
			},
			{"auth version 5 should be detected", "testdata/auth_version_5.bson", 5},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				restore := newRestoreDBUsersAndRoles()
				if tc.authVersionFile != "" {
					putAuthVersionIntent(restore, version, tc.authVersionFile)
				}

				got, err := restore.GetDumpAuthVersion()
				require.NoError(t, err, "should determine the auth version")
				assert.Equal(
					t,
					tc.expectedVersion,
					got,
					"should detect auth version %d",
					tc.expectedVersion,
				)
			})
		}

		t.Run(
			"without an authSchema document should error for dump server versions pre 8.1.0",
			func(t *testing.T) {
				restore := newRestoreDBUsersAndRoles()
				restore.dumpServerVersion = db.Version{8, 0, 0}
				putAuthVersionIntent(
					restore,
					version,
					"testdata/system.version.no_auth_schema.bson",
				)

				_, err := restore.GetDumpAuthVersion()
				require.Error(
					t,
					err,
					"should reject a dump with no authSchema document below server version 8.1.0",
				)
			},
		)

		t.Run(
			"without an authSchema document should detect auth version 5 for dump server version 8.1.0+",
			func(t *testing.T) {
				restore := newRestoreDBUsersAndRoles()
				restore.dumpServerVersion = db.Version{8, 1, 0}

				got, err := restore.GetDumpAuthVersion()
				require.NoError(t, err, "should determine the auth version")
				assert.Equal(
					t,
					5,
					got,
					"should default to auth version 5 for dump server version 8.1.0+",
				)
			},
		)
	})
}

// putAuthVersionIntent adds an intent for testdata/system.version at the
// given location, so GetDumpAuthVersion can read the authSchema document it
// contains.
func putAuthVersionIntent(restore *MongoRestore, version db.Version, location string) {
	intent := &intents.Intent{
		ServerVersion: version,
		DB:            "admin",
		C:             "system.version",
		Location:      location,
	}
	intent.BSONFile = &realBSONFile{
		path:   location,
		intent: intent,
	}
	restore.manager.Put(intent)
}

// newRestoreDBUsersAndRoles builds the MongoRestore fixture shared by the
// "using --restoreDbUsersAndRoles" scenarios.
func newRestoreDBUsersAndRoles() *MongoRestore {
	return &MongoRestore{
		InputOptions: &InputOptions{
			RestoreDBUsersAndRoles: true,
		},
		ToolOptions: &commonOpts.ToolOptions{
			Namespace: &commonOpts.Namespace{
				DB: "TestDB",
			},
		},
		manager: intents.NewIntentManager(),
	}
}

const indexCollationTestDataFile = "testdata/index_collation.json"

func TestIndexGetsSimpleCollation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	metadata, err := readCollationTestData(indexCollationTestDataFile)
	require.NoError(t, err, "should read the collation test data file")

	dumpDir := testDumpDir{
		dirName: "index_collation",
		collections: []testCollData{{
			ns:       "test.foo",
			metadata: metadata,
		}},
	}

	err = dumpDir.Create()
	require.NoError(t, err, "should create the dump directory")

	args := []string{
		DropOption,
		dumpDir.Path(),
	}
	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	result := restore.Restore()
	require.NoError(t, result.Err, "should restore the collection with its simple collation")
}

func TestAutoIndexIdHandling(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	type testCase struct {
		version                  db.Version
		isLocalDB                bool
		expectAutoIndexIDPresent bool
	}

	testCases := []testCase{
		{
			version:                  db.Version{7, 0, 0},
			isLocalDB:                false,
			expectAutoIndexIDPresent: true,
		},
		{
			version:                  db.Version{7, 0, 0},
			isLocalDB:                true,
			expectAutoIndexIDPresent: true,
		},
		{
			version:                  db.Version{8, 1, 0},
			expectAutoIndexIDPresent: true,
		},
		{
			version:                  db.Version{8, 2, 0},
			expectAutoIndexIDPresent: false,
		},
		{
			version:                  db.Version{9, 0, 0},
			expectAutoIndexIDPresent: false,
		},
	}

	for _, tc := range testCases {
		t.Run(
			fmt.Sprintf("autoIndexId handling with version %s", tc.version.String()),
			func(t *testing.T) {
				dbName := "foo"
				if tc.isLocalDB {
					dbName = "local"
				}
				restore := &MongoRestore{
					ToolOptions: &commonOpts.ToolOptions{
						Namespace: &commonOpts.Namespace{
							DB: dbName,
						},
					},
					serverVersion: tc.version,
				}

				origCollation := "en"
				options := bson.D{
					{"collation", "en"},
					{"autoIndexId", false},
				}

				options = restore.UpdateAutoIndexId(options)

				newCollation, err := bsonutil.FindStringValueByKey("collation", &options)
				require.NoError(t, err)

				require.Equal(
					t,
					origCollation,
					newCollation,
					"collation is preserved regardless of changes to `autoIndexId` field",
				)

				if tc.expectAutoIndexIDPresent {
					autoIndexId, err := bsonutil.FindValueByKey("autoIndexId", &options)
					require.NoError(t, err)

					if tc.isLocalDB {
						require.Equal(t, false, autoIndexId)
					} else {
						require.Equal(t, true, autoIndexId)
					}
				} else {
					_, err := bsonutil.FindValueByKey("autoIndexId", &options)
					require.Error(t, err)
				}
			},
		)
	}
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
