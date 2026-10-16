// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongorestore

import (
	"fmt"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/intents"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestGetDumpAuthVersion(t *testing.T) {
	restore := &MongoRestore{}

	version := db.Version{8, 0, 0}

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
					ServerVersion: version,
					DB:            "admin",
					C:             "system.version",
					Location:      "testdata/auth_version_3.bson",
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
					ServerVersion: version,
					DB:            "admin",
					C:             "system.version",
					Location:      "testdata/auth_version_5.bson",
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
					ServerVersion: version,
					DB:            "admin",
					C:             "system.version",
					Location:      "testdata/auth_version_3.bson",
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
					ServerVersion: version,
					DB:            "admin",
					C:             "system.version",
					Location:      "testdata/auth_version_5.bson",
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
						ServerVersion: version,
						DB:            "admin",
						C:             "system.version",
						Location:      "testdata/system.version.no_auth_schema.bson",
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

func TestAutoIndexIdHandling(t *testing.T) {
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
