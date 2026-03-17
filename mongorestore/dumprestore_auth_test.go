// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestDumpRestoreEnforcesAuthRoles verifies that mongodump requires the backup role and
// mongorestore requires the restore role, and that fine-grained custom roles correctly scope
// per-collection access.
func TestDumpRestoreEnforcesAuthRoles(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	const (
		dbName             = "testdr_enforces"
		backupUser         = "enforces_backup"
		restoreUser        = "enforces_restore"
		backupFooUser      = "enforces_backupfoo"
		restoreChesterUser = "enforces_restchester"
		restoreFooUser     = "enforces_restfoo"
	)

	adminClient, err := testutil.GetBareSession()
	require.NoError(t, err)
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)
	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err)

	adminDB := adminClient.Database("admin")
	testDB := adminClient.Database(dbName)
	fooColl := testDB.Collection("foo")

	cleanup := func() {
		silentDropUser(adminDB, backupUser)
		silentDropUser(adminDB, restoreUser)
		silentDropUser(adminDB, backupFooUser)
		silentDropUser(adminDB, restoreChesterUser)
		silentDropUser(adminDB, restoreFooUser)
		silentDropRole(testDB, "backupFoo")
		silentDropRole(testDB, "restoreChester")
		silentDropRole(testDB, "restoreFoo")
		_ = testDB.Drop(context.Background())
	}
	cleanup()
	t.Cleanup(cleanup)

	backupActions := bson.A{"find", "listCollections", "listIndexes"}
	restoreActions := bson.A{
		"collMod",
		"createCollection",
		"createIndex",
		"dropCollection",
		"find",
		"insert",
		"listCollections",
		"listIndexes",
	}

	if serverVersion.GTE(db.Version{8, 3, 0}) {
		backupActions = append(backupActions, "performRawDataOperations")
		restoreActions = append(restoreActions, "performRawDataOperations")
	}

	mustCreateUser(t, adminDB, backupUser, "password", bson.A{adminRole("backup")})
	mustCreateUser(t, adminDB, restoreUser, "password", bson.A{adminRole("restore")})
	mustCreateRole(t, testDB, "backupFoo", bson.A{
		privilege(dbName, "foo", backupActions),
		privilege(dbName, "", backupActions),
	}, bson.A{})
	mustCreateRole(t, testDB, "restoreChester", bson.A{
		privilege(dbName, "chester", restoreActions),
		privilege(dbName, "", bson.A{"listCollections", "listIndexes"}),
	}, bson.A{})
	mustCreateRole(t, testDB, "restoreFoo", bson.A{
		privilege(dbName, "foo", restoreActions),
		privilege(dbName, "", bson.A{"listCollections", "listIndexes"}),
	}, bson.A{})
	mustCreateUser(t, adminDB, backupFooUser, "password", bson.A{dbRole("backupFoo", dbName)})
	mustCreateUser(
		t,
		adminDB,
		restoreChesterUser,
		"password",
		bson.A{dbRole("restoreChester", dbName)},
	)
	mustCreateUser(t, adminDB, restoreFooUser, "password", bson.A{dbRole("restoreFoo", dbName)})

	sysUsersTotal := sysUsersWhere(t, adminClient, bson.D{})

	_, err = fooColl.InsertOne(context.Background(), bson.D{{Key: "a", Value: 22}})
	require.NoError(t, err)
	require.Equal(t, int64(1), docCount(t, fooColl))

	t.Run("full db requires backup and restore roles", func(t *testing.T) {
		dumpDir, cleanDump := testutil.MakeTempDir(t)
		defer cleanDump()

		dumpOpts := toolOptsForUser(t, backupUser, "password")
		dumpOpts.Namespace = &options.Namespace{DB: dbName}
		require.NoError(t, runDump(t, dumpOpts, dumpDir, nil), "dump with backup user")

		require.NoError(t, fooColl.Drop(context.Background()))

		noAuthOpts := toolOptsNoAuth(t)
		noAuthErr := runRestore(t, noAuthOpts, dumpDir, nil)
		// On MongoDB 4.2 with TLS, the client cert used in CI acts as an implicit intra-cluster
		// x.509 credential, allowing a "no auth" connection to succeed. This was tightened in 4.4,
		// so these assertions are only reliable on 4.4+.
		if serverVersion.GTE(db.Version{4, 4, 0}) {
			assert.ErrorContains(
				t,
				noAuthErr,
				"requires authentication",
				"restore without auth should fail",
			)
			assert.Equal(
				t,
				int64(0),
				docCount(t, fooColl),
				"restore without auth should not insert anything",
			)
		}

		restoreOpts := toolOptsForUser(t, restoreUser, "password")
		require.NoError(t, runRestore(t, restoreOpts, dumpDir, nil), "restore with restore user")
		assert.Equal(t, int64(1), docCount(t, fooColl))

		var doc bson.D
		require.NoError(t, fooColl.FindOne(context.Background(), bson.D{}).Decode(&doc))
		aVal, err := bsonutil.FindValueByKey("a", &doc)
		require.NoError(t, err)
		assert.Equal(t, int32(22), aVal)

		assert.Equal(
			t,
			sysUsersTotal,
			sysUsersWhere(t, adminClient, bson.D{}),
			"system users count should not change after restore",
		)
	})

	t.Run("per collection requires matching custom role", func(t *testing.T) {
		require.Equal(t, int64(1), docCount(t, fooColl),
			"fooColl must have 1 doc (set up by previous subtest)")

		dumpDir, cleanDump := testutil.MakeTempDir(t)
		defer cleanDump()

		backupFooOpts := toolOptsForUser(t, backupFooUser, "password")
		backupFooOpts.Namespace = &options.Namespace{DB: dbName, Collection: "foo"}
		require.NoError(
			t,
			runDump(t, backupFooOpts, dumpDir, nil),
			"dump foo collection with backupFoo user",
		)

		require.NoError(t, fooColl.Drop(context.Background()))

		bsonFilePath := filepath.Join(dumpDir, dbName, "foo.bson")

		restoreChesterOpts := toolOptsForUser(t, restoreChesterUser, "password")
		if serverVersion.GTE(db.Version{4, 4, 0}) {
			assert.ErrorContains(
				t,
				runRestore(t, restoreChesterOpts, bsonFilePath, func(o *Options) {
					o.ToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "foo"}
				}),
				"not authorized",
				"restore with wrong-collection user should fail",
			)
			assert.Equal(
				t,
				int64(0),
				docCount(t, fooColl),
				"restore with wrong-collection user should not insert anything",
			)
		}

		restoreFooOpts := toolOptsForUser(t, restoreFooUser, "password")
		require.NoError(
			t,
			runRestore(
				t,
				restoreFooOpts,
				bsonFilePath,
				func(o *Options) { o.ToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "foo"} },
			),
			"restore foo with restoreFoo user",
		)
		assert.Equal(t, int64(1), docCount(t, fooColl))

		var doc bson.D
		require.NoError(t, fooColl.FindOne(context.Background(), bson.D{}).Decode(&doc))
		aVal, err := bsonutil.FindValueByKey("a", &doc)
		require.NoError(t, err)
		assert.Equal(t, int32(22), aVal)

		assert.Equal(
			t,
			sysUsersTotal,
			sysUsersWhere(t, adminClient, bson.D{}),
			"system users count should not change after restore",
		)
	})
}

// TestDumpRestorePreservesAdminUsersAndRoles verifies that user and role definitions stored in the
// admin database survive a round-trip through mongodump and mongorestore, including with --drop.
func TestDumpRestorePreservesAdminUsersAndRoles(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	t.Run("backup and restore roles", func(t *testing.T) {
		dumpRestorePreservesAdminUsersAndRoles(t, "backup", "restore")
	})
	t.Run("root roles", func(t *testing.T) {
		dumpRestorePreservesAdminUsersAndRoles(t, "root", "root")
	})
}

func dumpRestorePreservesAdminUsersAndRoles(t *testing.T, backupRole, restoreRole string) {
	t.Helper()

	const (
		fooDBName   = "testdr_preserves"
		rootUser    = "preserves_root"
		backupUser  = "preserves_backup"
		restoreUser = "preserves_restore"
		testUser    = "preserves_test"
		customRole  = "preserves_custom"
		root2User   = "preserves_root2"   // used in drop_restore_overrides_mutations subtest
		customRole2 = "preserves_custom2" // used in drop_restore_overrides_mutations subtest
	)

	adminClient, err := testutil.GetBareSession()
	require.NoError(t, err)

	adminDB := adminClient.Database("admin")
	fooDB := adminClient.Database(fooDBName)
	fooColl := fooDB.Collection("foo")

	cleanup := func() {
		silentDropUser(adminDB, rootUser)
		silentDropUser(adminDB, backupUser)
		silentDropUser(adminDB, restoreUser)
		silentDropUser(adminDB, testUser)
		silentDropUser(adminDB, root2User)
		silentDropRole(adminDB, customRole)
		silentDropRole(adminDB, customRole2)
		_ = fooDB.Drop(context.Background())
	}
	cleanup()
	t.Cleanup(cleanup)

	mustCreateUser(t, adminDB, rootUser, "pass", bson.A{adminRole("root")})
	mustCreateUser(t, adminDB, backupUser, "pass", bson.A{adminRole(backupRole)})
	mustCreateUser(t, adminDB, restoreUser, "pass", bson.A{adminRole(restoreRole)})
	mustCreateRole(
		t,
		adminDB,
		customRole,
		bson.A{privilege(fooDBName, "foo", bson.A{"find"})},
		bson.A{},
	)
	mustCreateUser(t, adminDB, testUser, "pass", bson.A{dbRole(customRole, "admin")})

	_, err = fooColl.InsertOne(context.Background(), bson.D{{Key: "word", Value: "tomato"}})
	require.NoError(t, err)

	require.Equal(
		t,
		int64(1),
		sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: rootUser}}),
		"rootUser setup",
	)
	require.Equal(
		t,
		int64(1),
		sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: backupUser}}),
		"backupUser setup",
	)
	require.Equal(
		t,
		int64(1),
		sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: restoreUser}}),
		"restoreUser setup",
	)
	require.Equal(
		t,
		int64(1),
		sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: testUser}}),
		"testUser setup",
	)
	require.Equal(
		t,
		int64(1),
		sysRolesWhere(t, adminClient, bson.D{{Key: "role", Value: customRole}}),
		"customRole setup",
	)

	dumpDir, cleanDump := testutil.MakeTempDir(t)
	t.Cleanup(cleanDump)

	dumpOpts := toolOptsForUser(t, backupUser, "pass")
	require.NoError(t, runDump(t, dumpOpts, dumpDir, nil))

	require.NoError(t, fooDB.Drop(context.Background()))
	silentDropUser(adminDB, backupUser)
	silentDropUser(adminDB, testUser)
	silentDropRole(adminDB, customRole)

	require.Equal(
		t,
		int64(0),
		sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: backupUser}}),
		"backupUser dropped",
	)
	require.Equal(
		t,
		int64(0),
		sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: testUser}}),
		"testUser dropped",
	)
	require.Equal(
		t,
		int64(0),
		sysRolesWhere(t, adminClient, bson.D{{Key: "role", Value: customRole}}),
		"customRole dropped",
	)
	require.Equal(t, int64(0), docCount(t, fooColl), "foo coll dropped")

	restoreOpts := toolOptsForUser(t, restoreUser, "pass")

	t.Run("restore_preserves_users_roles_and_data", func(t *testing.T) {
		require.NoError(t, runRestore(t, restoreOpts, dumpDir, nil))

		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: rootUser}}),
			"rootUser restored",
		)
		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: backupUser}}),
			"backupUser restored",
		)
		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: restoreUser}}),
			"restoreUser restored",
		)
		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: testUser}}),
			"testUser restored",
		)
		assert.Equal(
			t,
			int64(1),
			sysRolesWhere(t, adminClient, bson.D{{Key: "role", Value: customRole}}),
			"customRole restored",
		)
		assert.Equal(t, int64(1), docCount(t, fooColl), "foo data restored")
	})

	t.Run("drop_restore_overrides_mutations", func(t *testing.T) {
		mustCreateUser(t, adminDB, root2User, "pass", bson.A{adminRole("root")})
		silentDropRole(adminDB, customRole)
		mustCreateRole(t, adminDB, customRole2, bson.A{}, bson.A{})
		silentDropUser(adminDB, rootUser)

		require.NoError(
			t,
			runRestore(t, restoreOpts, dumpDir, func(o *Options) { o.OutputOptions.Drop = true }),
		)

		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: rootUser}}),
			"rootUser re-restored",
		)
		assert.Equal(
			t,
			int64(0),
			sysUsersWhere(t, adminClient, bson.D{{Key: "user", Value: root2User}}),
			"root2User dropped",
		)
		assert.Equal(
			t,
			int64(0),
			sysRolesWhere(t, adminClient, bson.D{{Key: "role", Value: customRole2}}),
			"customRole2 dropped",
		)
		assert.Equal(
			t,
			int64(1),
			sysRolesWhere(t, adminClient, bson.D{{Key: "role", Value: customRole}}),
			"customRole re-restored",
		)
	})
}

// TestDumpRestoreSingleDBWithDBUsersAndRoles verifies that the --dumpDbUsersAndRoles and
// --restoreDbUsersAndRoles flags correctly include or exclude per-database user and role
// definitions during single-database dump/restore operations.
func TestDumpRestoreSingleDBWithDBUsersAndRoles(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	t.Run("first_run", func(t *testing.T) {
		dumpRestoreSingleDBWithUsersAndRoles(t)
	})
	t.Run("second_run", func(t *testing.T) {
		dumpRestoreSingleDBWithUsersAndRoles(t)
	})
}

func dumpRestoreSingleDBWithUsersAndRoles(t *testing.T) {
	t.Helper()

	const (
		fooDBName          = "testdr_singledb_foo"
		barDBName          = "testdr_singledb_bar"
		rootUser           = "singledb_root"
		backupUser         = "singledb_backup"
		restoreUser        = "singledb_restore"
		dummyRole          = "singledb_dummy"
		fooUser            = "singledb_foouser"
		fooRole            = "singledb_footole"
		backupFooChestRole = "singledb_foochest_role"
		backupFooChestUser = "singledb_foochest_user"
		fooUser2           = "singledb_foouser2"  // used in drop_restore_overrides subtest
		fooRole2           = "singledb_footole2"  // used in drop_restore_overrides subtest
		barUserName        = "singledb_baruser"   // used in admin_db subtest
		adminUserName      = "singledb_adminuser" // used in admin_db subtest
		barUser2Name       = "singledb_baruser2"  // used in admin_db subtest
	)

	adminClient, err := testutil.GetBareSession()
	require.NoError(t, err)

	adminDB := adminClient.Database("admin")
	fooDB := adminClient.Database(fooDBName)
	barDB := adminClient.Database(barDBName)

	cleanup := func() {
		silentDropUser(adminDB, rootUser)
		silentDropUser(adminDB, backupUser)
		silentDropUser(adminDB, restoreUser)
		silentDropUser(adminDB, adminUserName)
		silentDropRole(adminDB, dummyRole)
		silentDropUser(fooDB, fooUser)
		silentDropUser(fooDB, fooUser2)
		silentDropUser(fooDB, backupFooChestUser)
		silentDropRole(fooDB, fooRole)
		silentDropRole(fooDB, fooRole2)
		silentDropRole(fooDB, backupFooChestRole)
		silentDropUser(barDB, barUserName)
		silentDropUser(barDB, barUser2Name)
		_ = fooDB.Drop(context.Background())
		_ = barDB.Drop(context.Background())
	}
	cleanup()
	t.Cleanup(cleanup)

	mustCreateUser(t, adminDB, rootUser, "pass", bson.A{adminRole("root")})
	mustCreateUser(t, adminDB, backupUser, "pass", bson.A{adminRole("backup")})
	mustCreateUser(t, adminDB, restoreUser, "pass", bson.A{adminRole("restore")})
	mustCreateRole(t, adminDB, dummyRole, bson.A{}, bson.A{})
	mustCreateUser(t, fooDB, fooUser, "pass", bson.A{dbRole("readWrite", fooDBName)})
	mustCreateRole(t, fooDB, fooRole, bson.A{}, bson.A{})
	mustCreateRole(
		t,
		fooDB,
		backupFooChestRole,
		bson.A{privilege(fooDBName, "chester", bson.A{"find"})},
		bson.A{},
	)
	mustCreateUser(
		t,
		fooDB,
		backupFooChestUser,
		"pass",
		bson.A{dbRole(backupFooChestRole, fooDBName)},
	)

	sysVersionCountInitial, err := adminClient.Database("admin").
		Collection("system.version").
		CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err)

	var versionDoc bson.D
	require.NoError(
		t,
		adminClient.Database("admin").
			Collection("system.version").
			FindOne(context.Background(), bson.D{}).
			Decode(&versionDoc),
	)

	barColl := fooDB.Collection("bar")
	_, err = barColl.InsertOne(context.Background(), bson.D{{Key: "a", Value: 1}})
	require.NoError(t, err)
	require.Equal(t, int64(1), docCount(t, barColl))

	// dumpDir is shared across phases 1–4 (foo DB dumps).
	dumpDir, cleanDump := testutil.MakeTempDir(t)
	t.Cleanup(cleanDump)

	// Set by phase 1, read by phases 3–5.
	var fooUserCountAfterRecreate, fooRoleCountAfterRecreate int64

	t.Run("dump without user data omits users and roles", func(t *testing.T) {
		opts := baseToolOpts(t)
		opts.Namespace = &options.Namespace{DB: fooDBName}
		require.NoError(t, runDump(t, opts, dumpDir, nil))

		require.NoError(t, fooDB.Drop(context.Background()))
		require.NoError(t, fooDB.RunCommand(context.Background(),
			bson.D{{Key: "dropAllUsersFromDatabase", Value: 1}}).Err())
		require.NoError(t, fooDB.RunCommand(context.Background(),
			bson.D{{Key: "dropAllRolesFromDatabase", Value: 1}}).Err())

		restoreOpts := baseToolOpts(t)
		require.NoError(
			t,
			runRestore(
				t,
				restoreOpts,
				filepath.Join(dumpDir, fooDBName),
				func(o *Options) { o.ToolOptions.Namespace = &options.Namespace{DB: fooDBName} },
			),
		)

		assert.Equal(t, int64(1), docCount(t, barColl), "data restored")
		assert.Equal(
			t,
			int64(0),
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"no users when dump had no user data",
		)
		assert.Equal(
			t,
			int64(0),
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"no roles when dump had no user data",
		)

		// Re-create foo user and role for subsequent phases.
		mustCreateUser(t, fooDB, fooUser, "pass", bson.A{dbRole("readWrite", fooDBName)})
		mustCreateRole(t, fooDB, fooRole, bson.A{}, bson.A{})
		fooUserCountAfterRecreate = sysUsersWhere(
			t,
			adminClient,
			bson.D{{Key: "db", Value: fooDBName}},
		)
		fooRoleCountAfterRecreate = sysRolesWhere(
			t,
			adminClient,
			bson.D{{Key: "db", Value: fooDBName}},
		)
	})

	t.Run("dump with flag restore without flag ignores user data", func(t *testing.T) {
		opts := baseToolOpts(t)
		opts.Namespace = &options.Namespace{DB: fooDBName}
		require.NoError(
			t,
			runDump(
				t,
				opts,
				dumpDir,
				func(d *mongodump.MongoDump) { d.OutputOptions.DumpDBUsersAndRoles = true },
			),
		)

		require.NoError(t, fooDB.Drop(context.Background()))
		silentDropUser(fooDB, fooUser)
		silentDropUser(fooDB, backupFooChestUser)
		silentDropRole(fooDB, fooRole)
		silentDropRole(fooDB, backupFooChestRole)

		require.Equal(
			t,
			int64(0),
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
		)
		require.Equal(
			t,
			int64(0),
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
		)

		restoreOpts := baseToolOpts(t)
		require.NoError(
			t,
			runRestore(
				t,
				restoreOpts,
				filepath.Join(dumpDir, fooDBName),
				func(o *Options) { o.ToolOptions.Namespace = &options.Namespace{DB: fooDBName} },
			),
		)

		assert.Equal(t, int64(1), docCount(t, barColl), "data restored")
		assert.Equal(
			t,
			int64(0),
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"no users when --restoreDbUsersAndRoles omitted",
		)
		assert.Equal(
			t,
			int64(0),
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"no roles when --restoreDbUsersAndRoles omitted",
		)
	})

	t.Run("restore with flag includes users and roles", func(t *testing.T) {
		restoreOpts := baseToolOpts(t)
		require.NoError(
			t,
			runRestore(t, restoreOpts, filepath.Join(dumpDir, fooDBName), func(o *Options) {
				o.ToolOptions.Namespace = &options.Namespace{DB: fooDBName}
				o.InputOptions.RestoreDBUsersAndRoles = true
			}),
		)

		assert.Equal(t, int64(1), docCount(t, barColl), "data present")
		assert.Equal(
			t,
			fooUserCountAfterRecreate,
			sysUsersWhere(
				t,
				adminClient,
				bson.D{{Key: "db", Value: fooDBName}},
			),
			"foo users restored",
		)
		assert.Equal(
			t,
			fooRoleCountAfterRecreate,
			sysRolesWhere(
				t,
				adminClient,
				bson.D{{Key: "db", Value: fooDBName}},
			),
			"foo roles restored",
		)

		var versionDocNow bson.D
		require.NoError(
			t,
			adminClient.Database("admin").
				Collection("system.version").
				FindOne(context.Background(), bson.D{}).
				Decode(&versionDocNow),
		)
		assert.Equal(t, versionDoc, versionDocNow, "version doc unchanged")
	})

	t.Run("drop restore overrides user and role mutations", func(t *testing.T) {
		silentDropUser(fooDB, fooUser)
		mustCreateUser(t, fooDB, fooUser2, "password", bson.A{dbRole("readWrite", fooDBName)})
		silentDropRole(fooDB, fooRole)
		mustCreateRole(t, fooDB, fooRole2, bson.A{}, bson.A{})

		restoreOpts := baseToolOpts(t)
		require.NoError(
			t,
			runRestore(t, restoreOpts, filepath.Join(dumpDir, fooDBName), func(o *Options) {
				o.ToolOptions.Namespace = &options.Namespace{DB: fooDBName}
				o.InputOptions.RestoreDBUsersAndRoles = true
				o.OutputOptions.Drop = true
			}),
		)

		assert.Equal(t, int64(1), docCount(t, barColl), "data restored")
		assert.Equal(
			t,
			fooUserCountAfterRecreate,
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"foo users",
		)
		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(
				t,
				adminClient,
				bson.D{{Key: "user", Value: fooUser}, {Key: "db", Value: fooDBName}},
			),
			"original fooUser restored",
		)
		assert.Equal(
			t,
			fooRoleCountAfterRecreate,
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"foo roles",
		)
		assert.Equal(
			t,
			int64(1),
			sysRolesWhere(
				t,
				adminClient,
				bson.D{{Key: "role", Value: fooRole}, {Key: "db", Value: fooDBName}},
			),
			"original fooRole restored",
		)

		var versionDocNow bson.D
		require.NoError(t, adminClient.Database("admin").Collection("system.version").
			FindOne(context.Background(), bson.D{}).Decode(&versionDocNow))
		assert.Equal(t, versionDoc, versionDocNow, "version doc unchanged")
	})

	t.Run("admin db dump captures cross db users and roles", func(t *testing.T) {
		mustCreateUser(t, barDB, barUserName, "pwd", bson.A{})
		mustCreateUser(t, adminDB, adminUserName, "pwd", bson.A{})

		adminUserCountForDump := sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: "admin"}})

		adminDumpDir, cleanAdminDump := testutil.MakeTempDir(t)
		defer cleanAdminDump()

		opts := baseToolOpts(t)
		opts.Namespace = &options.Namespace{DB: "admin"}
		require.NoError(t, runDump(t, opts, adminDumpDir, nil))

		mustCreateUser(t, barDB, barUser2Name, "pwd", bson.A{})
		silentDropUser(fooDB, fooUser)
		// Drop only the test users we own in admin — never all admin users, as that
		// would delete the mongod admin account and break subsequent connections.
		silentDropUser(adminDB, rootUser)
		silentDropUser(adminDB, backupUser)
		silentDropUser(adminDB, restoreUser)
		silentDropUser(adminDB, adminUserName)

		restoreOpts := baseToolOpts(t)
		require.NoError(
			t,
			runRestore(t, restoreOpts, filepath.Join(adminDumpDir, "admin"), func(o *Options) {
				o.ToolOptions.Namespace = &options.Namespace{DB: "admin"}
				o.OutputOptions.Drop = true
			}),
		)

		assert.Equal(
			t,
			fooUserCountAfterRecreate,
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"foo users restored via admin db restore",
		)
		assert.Equal(
			t,
			fooRoleCountAfterRecreate,
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"foo roles restored via admin db restore",
		)
		assert.Equal(
			t,
			int64(1),
			sysUsersWhere(
				t,
				adminClient,
				bson.D{{Key: "user", Value: barUserName}},
			),
			"bar user restored",
		)
		assert.Equal(
			t,
			int64(0),
			sysUsersWhere(
				t,
				adminClient,
				bson.D{{Key: "user", Value: barUser2Name}},
			),
			"bar user2 dropped",
		)
		assert.Equal(
			t,
			adminUserCountForDump,
			sysUsersWhere(
				t,
				adminClient,
				bson.D{{Key: "db", Value: "admin"}},
			),
			"admin users restored",
		)

		var versionDocNow bson.D
		require.NoError(
			t,
			adminClient.Database("admin").
				Collection("system.version").
				FindOne(context.Background(), bson.D{}).
				Decode(&versionDocNow),
		)
		assert.Equal(t, versionDoc, versionDocNow, "version doc unchanged")
	})

	t.Run("full dump restore preserves all users and roles", func(t *testing.T) {
		allDumpDir, cleanAllDump := testutil.MakeTempDir(t)
		defer cleanAllDump()

		// Capture counts right before the dump so assertions reflect actual state.
		fooUserCount := sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}})
		fooRoleCount := sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}})
		adminUserCount := sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: "admin"}})
		adminRoleCount := sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: "admin"}})

		opts := baseToolOpts(t)
		opts.Namespace = &options.Namespace{}
		require.NoError(t, runDump(t, opts, allDumpDir, nil))

		require.NoError(t, fooDB.Drop(context.Background()))
		silentDropUser(fooDB, fooUser)
		silentDropUser(fooDB, backupFooChestUser)
		silentDropRole(fooDB, fooRole)
		silentDropRole(fooDB, backupFooChestRole)

		require.Equal(
			t,
			int64(0),
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
		)
		require.Equal(
			t,
			int64(0),
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
		)

		restoreOpts := baseToolOpts(t)
		require.NoError(t, runRestore(t, restoreOpts, allDumpDir, nil))

		assert.Equal(t, int64(1), docCount(t, barColl), "data restored")
		assert.Equal(
			t,
			fooUserCount,
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"foo users",
		)
		assert.Equal(
			t,
			fooRoleCount,
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: fooDBName}}),
			"foo roles",
		)
		assert.Equal(
			t,
			adminUserCount,
			sysUsersWhere(t, adminClient, bson.D{{Key: "db", Value: "admin"}}),
			"admin users",
		)
		assert.Equal(
			t,
			adminRoleCount,
			sysRolesWhere(t, adminClient, bson.D{{Key: "db", Value: "admin"}}),
			"admin roles",
		)

		sysVersionCountNow, err := adminClient.Database("admin").
			Collection("system.version").
			CountDocuments(context.Background(), bson.D{})
		require.NoError(t, err)
		assert.Equal(
			t,
			sysVersionCountInitial,
			sysVersionCountNow,
			"system.version count unchanged",
		)

		var versionDocNow bson.D
		require.NoError(
			t,
			adminClient.Database("admin").
				Collection("system.version").
				FindOne(context.Background(), bson.D{}).
				Decode(&versionDocNow),
		)
		assert.Equal(t, versionDoc, versionDocNow, "version doc unchanged")
	})
}

// baseToolOpts returns a fresh set of tool options from the environment.
func baseToolOpts(t *testing.T) *options.ToolOptions {
	t.Helper()
	opts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	return opts
}

// toolOptsForUser returns a copy of the environment's tool options with
// authentication overridden to use the given username and password.
func toolOptsForUser(t *testing.T, username, password string) *options.ToolOptions {
	t.Helper()
	opts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	opts.Auth = &options.Auth{
		Username: username,
		Password: password,
		Source:   "admin",
	}
	return opts
}

// toolOptsNoAuth returns a copy of the environment's tool options with
// authentication cleared. Connecting to an auth-required server will fail.
func toolOptsNoAuth(t *testing.T) *options.ToolOptions {
	t.Helper()
	opts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	opts.Auth = &options.Auth{}
	return opts
}

// runDump builds and executes a MongoDump with the given tool options and output directory.
// configure is an optional callback to set additional fields before Init is called.
func runDump(
	t *testing.T,
	toolOpts *options.ToolOptions,
	outDir string,
	configure func(*mongodump.MongoDump),
) error {
	t.Helper()
	dump := &mongodump.MongoDump{
		ToolOptions:  toolOpts,
		InputOptions: &mongodump.InputOptions{},
		OutputOptions: &mongodump.OutputOptions{
			Out:                    outDir,
			NumParallelCollections: 1,
		},
	}
	if configure != nil {
		configure(dump)
	}
	if err := dump.Init(); err != nil {
		return err
	}
	return dump.Dump()
}

// runRestore builds and executes a MongoRestore with the given tool options and target directory.
// configure is an optional callback to modify the Options before New is called.
func runRestore(
	t *testing.T,
	toolOpts *options.ToolOptions,
	targetDir string,
	configure func(*Options),
) error {
	t.Helper()
	opts := Options{
		ToolOptions:  toolOpts,
		InputOptions: &InputOptions{},
		NSOptions:    &NSOptions{},
		OutputOptions: &OutputOptions{
			NumInsertionWorkers: 1,
			// When mongorestore is run, these fields end up set to these default values by our flag
			// parsing. But since we're skipping that, we need to set the default manually here.
			TempUsersColl: "tempusers",
			TempRolesColl: "temproles",
		},
		TargetDirectory: targetDir,
	}
	if configure != nil {
		configure(&opts)
	}
	restore, err := New(opts)
	if err != nil {
		return err
	}
	defer restore.Close()
	return restore.Restore().Err
}

// sysUsersWhere counts documents in admin.system.users matching filter.
func sysUsersWhere(t *testing.T, client *mongo.Client, filter bson.D) int64 {
	t.Helper()
	count, err := client.Database("admin").Collection("system.users").
		CountDocuments(context.Background(), filter)
	require.NoError(t, err)
	return count
}

// sysRolesWhere counts documents in admin.system.roles matching filter.
func sysRolesWhere(t *testing.T, client *mongo.Client, filter bson.D) int64 {
	t.Helper()
	count, err := client.Database("admin").Collection("system.roles").
		CountDocuments(context.Background(), filter)
	require.NoError(t, err)
	return count
}

// docCount returns the number of documents in the collection.
func docCount(t *testing.T, coll *mongo.Collection) int64 {
	t.Helper()
	count, err := coll.CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err)
	return count
}

// mustCreateUser runs a createUser command on db, failing the test on error.
func mustCreateUser(t *testing.T, db *mongo.Database, user, password string, roles bson.A) {
	t.Helper()
	require.NoError(t, db.RunCommand(context.Background(), bson.D{
		{Key: "createUser", Value: user},
		{Key: "pwd", Value: password},
		{Key: "roles", Value: roles},
	}).Err(), "createUser %q", user)
}

// mustCreateRole runs a createRole command on db, failing the test on error.
func mustCreateRole(
	t *testing.T,
	db *mongo.Database,
	role string,
	privileges bson.A,
	roles bson.A,
) {
	t.Helper()
	require.NoError(t, db.RunCommand(context.Background(), bson.D{
		{Key: "createRole", Value: role},
		{Key: "privileges", Value: privileges},
		{Key: "roles", Value: roles},
	}).Err(), "createRole %q", role)
}

// silentDropUser drops a user, ignoring errors (e.g. user not found).
func silentDropUser(db *mongo.Database, user string) {
	_ = db.RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: user}}).Err()
}

// silentDropRole drops a role, ignoring errors (e.g. role not found).
func silentDropRole(db *mongo.Database, role string) {
	_ = db.RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: role}}).Err()
}

// privilege returns a BSON document representing a single privilege entry.
func privilege(dbName, collection string, actions bson.A) bson.D {
	return bson.D{
		{Key: "resource", Value: bson.D{
			{Key: "db", Value: dbName},
			{Key: "collection", Value: collection},
		}},
		{Key: "actions", Value: actions},
	}
}

// adminRole returns a bson.D representing a role granted from the admin database.
func adminRole(role string) bson.D {
	return bson.D{{Key: "role", Value: role}, {Key: "db", Value: "admin"}}
}

// dbRole returns a bson.D representing a role granted from a specific database.
func dbRole(role, db string) bson.D {
	return bson.D{{Key: "role", Value: role}, {Key: "db", Value: db}}
}
