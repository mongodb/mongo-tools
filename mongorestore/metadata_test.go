// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/intents"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const ExistsDB = "restore_collection_exists"

func TestMongoRestoreConnectedToAtlasProxy(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

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
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test mongorestore", t, func() {
		sessionProvider, _, err := testutil.GetBareSessionProvider()
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

func TestGetDumpAuthVersion(t *testing.T) {

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	restore := &MongoRestore{}

	Convey("With a test mongorestore", t, func() {
		Convey("and no --restoreDbUsersAndRoles", func() {
			restore = &MongoRestore{
				InputOptions: &InputOptions{},
				ToolOptions:  &commonOpts.ToolOptions{},
				NSOptions:    &NSOptions{},
			}
			Convey("auth version 1 should be detected", func() {
				restore.manager = intents.NewIntentManager()
				version, err := restore.GetDumpAuthVersion()
				So(err, ShouldBeNil)
				So(version, ShouldEqual, 1)
			})

			Convey("auth version 3 should be detected", func() {
				restore.manager = intents.NewIntentManager()
				intent := &intents.Intent{
					DB:       "admin",
					C:        "system.version",
					Location: "testdata/auth_version_3.bson",
				}
				intent.BSONFile = &realBSONFile{
					path:   "testdata/auth_version_3.bson",
					intent: intent,
				}
				restore.manager.Put(intent)
				version, err := restore.GetDumpAuthVersion()
				So(err, ShouldBeNil)
				So(version, ShouldEqual, 3)
			})

			Convey("auth version 5 should be detected", func() {
				restore.manager = intents.NewIntentManager()
				intent := &intents.Intent{
					DB:       "admin",
					C:        "system.version",
					Location: "testdata/auth_version_5.bson",
				}
				intent.BSONFile = &realBSONFile{
					path:   "testdata/auth_version_5.bson",
					intent: intent,
				}
				restore.manager.Put(intent)
				version, err := restore.GetDumpAuthVersion()
				So(err, ShouldBeNil)
				So(version, ShouldEqual, 5)
			})
		})

		Convey("using --restoreDbUsersAndRoles", func() {
			restore = &MongoRestore{
				InputOptions: &InputOptions{
					RestoreDBUsersAndRoles: true,
				},
				ToolOptions: &commonOpts.ToolOptions{
					Namespace: &commonOpts.Namespace{
						DB: "TestDB",
					},
				},
			}

			Convey("auth version 3 should be detected when no file exists", func() {
				restore.manager = intents.NewIntentManager()
				version, err := restore.GetDumpAuthVersion()
				So(err, ShouldBeNil)
				So(version, ShouldEqual, 3)
			})

			Convey("auth version 3 should be detected when a version 3 file exists", func() {
				restore.manager = intents.NewIntentManager()
				intent := &intents.Intent{
					DB:       "admin",
					C:        "system.version",
					Location: "testdata/auth_version_3.bson",
				}
				intent.BSONFile = &realBSONFile{
					path:   "testdata/auth_version_3.bson",
					intent: intent,
				}
				restore.manager.Put(intent)
				version, err := restore.GetDumpAuthVersion()
				So(err, ShouldBeNil)
				So(version, ShouldEqual, 3)
			})

			Convey("auth version 5 should be detected", func() {
				restore.manager = intents.NewIntentManager()
				intent := &intents.Intent{
					DB:       "admin",
					C:        "system.version",
					Location: "testdata/auth_version_5.bson",
				}
				intent.BSONFile = &realBSONFile{
					path:   "testdata/auth_version_5.bson",
					intent: intent,
				}
				restore.manager.Put(intent)
				version, err := restore.GetDumpAuthVersion()
				So(err, ShouldBeNil)
				So(version, ShouldEqual, 5)
			})

			Convey("when system.version does not contain authSchema document", func() {
				Convey("should return an error for dump server versions pre 8.1.0", func() {
					restore.dumpServerVersion = db.Version{8, 0, 0}
					restore.manager = intents.NewIntentManager()
					intent := &intents.Intent{
						DB:       "admin",
						C:        "system.version",
						Location: "testdata/system.version.no_auth_schema.bson",
					}
					intent.BSONFile = &realBSONFile{
						path:   "testdata/system.version.no_auth_schema.bson",
						intent: intent,
					}
					restore.manager.Put(intent)
					_, err := restore.GetDumpAuthVersion()
					So(err, ShouldNotBeNil)
				})

				Convey("auth version 5 should be detected for dump server version 8.1.0+", func() {
					restore.dumpServerVersion = db.Version{8, 1, 0}
					version, err := restore.GetDumpAuthVersion()
					So(err, ShouldBeNil)
					So(version, ShouldEqual, 5)
				})
			})
		})
	})

}

const indexCollationTestDataFile = "testdata/index_collation.json"

func TestIndexGetsSimpleCollation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

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
