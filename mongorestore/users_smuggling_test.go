// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/intents"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongorestore/ns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDumpDir creates a dump directory containing the given files, each
// relative to the dump root (e.g. "vendordata/c1.bson"), and returns its path.
// The contents don't matter: intent creation only stats these files.
func writeDumpDir(t *testing.T, files ...string) string {
	t.Helper()

	root := t.TempDir()
	for _, file := range files {
		full := filepath.Join(root, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte{}, 0644))
	}

	return root
}

// newRestoreWithNamespaces builds a MongoRestore configured as
// ParseAndValidateOptions would for the given --db and --nsExclude values.
func newRestoreWithNamespaces(t *testing.T, db string, nsExclude []string) *MongoRestore {
	t.Helper()

	includes := []string{"*"}
	if db != "" {
		includes = []string{ns.Escape(db) + ".*"}
	}

	includer, err := ns.NewMatcher(includes)
	require.NoError(t, err)
	excluder, err := ns.NewMatcher(nsExclude)
	require.NoError(t, err)
	renamer, err := ns.NewRenamer(nil, nil)
	require.NoError(t, err)

	return &MongoRestore{
		manager:      intents.NewIntentManager(),
		InputOptions: &InputOptions{},
		ToolOptions:  &commonOpts.ToolOptions{Namespace: &commonOpts.Namespace{DB: db}},
		NSOptions:    &NSOptions{NSExclude: nsExclude},
		renamer:      renamer,
		includer:     includer,
		excluder:     excluder,
	}
}

// popAllIntents drains the manager and returns every intent it produced.
func popAllIntents(mr *MongoRestore) []*intents.Intent {
	mr.manager.Finalize(intents.Legacy)

	var all []*intents.Intent
	for intent := mr.manager.Pop(); intent != nil; intent = mr.manager.Pop() {
		all = append(all, intent)
	}

	return all
}

// TestSmuggledUsersFileIsNotRestoredAsUsers covers the privilege-escalation
// path where a dump directory for an ordinary database carries a file whose
// name decodes to a "$admin.system.*" collection. Because
// Intent.IsUsers/IsRoles key off that collection name, such a file would be
// merged into admin.system.users on the destination — bypassing an operator's
// `--nsExclude 'admin.*'`, since the namespace checked against the excluder is
// the ordinary database's, not admin's.
func TestSmuggledUsersFileIsNotRestoredAsUsers(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// Each of these lands in vendordata/ during a full, multi-db restore.
	smuggledFiles := []string{
		// The timeseries bucket prefix is stripped from the destination
		// collection name, so it survives as $admin.system.users.
		"system.buckets.$admin.system.users.bson",
		"system.buckets.$admin.system.roles.bson",
		"system.buckets.$admin.system.version.bson",

		// mongodump percent-escapes "$" in file names, and mongorestore
		// unescapes before matching, so this is the same attack.
		"system.buckets.%24admin.system.users.bson",

		// The bare form, which predates the timeseries trick.
		"$admin.system.users.bson",
		"$admin.system.roles.bson",
	}

	for _, file := range smuggledFiles {
		t.Run(file, func(t *testing.T) {
			root := writeDumpDir(t, filepath.Join("vendordata", file), "vendordata/c1.bson")

			mr := newRestoreWithNamespaces(t, "", []string{"admin.*"})
			target, err := newActualPath(root)
			require.NoError(t, err)

			// Either rejecting the dump or ignoring the file is acceptable;
			// what must never happen is the file becoming a users/roles intent.
			err = mr.CreateAllIntents(target)

			assert.Nil(
				t,
				mr.manager.Users(),
				"smuggled %s should not become the users intent",
				file,
			)
			assert.Nil(
				t,
				mr.manager.Roles(),
				"smuggled %s should not become the roles intent",
				file,
			)

			// Rejection aborts the whole restore, so intents for files read
			// before the offending one are never acted on.
			if err != nil {
				return
			}

			for _, intent := range popAllIntents(mr) {
				assert.False(
					t,
					intent.IsSpecialCollection(),
					"intent %s.%s from smuggled %s should not be a special collection",
					intent.DB,
					intent.C,
					file,
				)
				assert.False(
					t,
					intent.IsUsers() || intent.IsRoles(),
					"intent %s.%s from smuggled %s should not be a users or roles intent",
					intent.DB,
					intent.C,
					file,
				)
			}
		})
	}
}

// TestSmuggledUsersFileIsRejected pins the stricter half of the fix: a name
// that could only have been hand-assembled — a system.buckets. prefix wrapping
// an admin system collection — is a corrupt or hostile dump, and mongorestore
// should refuse it rather than silently drop the file.
func TestSmuggledUsersFileIsRejected(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(
		t,
		"vendordata/system.buckets.$admin.system.users.bson",
		"vendordata/c1.bson",
	)

	mr := newRestoreWithNamespaces(t, "", []string{"admin.*"})
	target, err := newActualPath(root)
	require.NoError(t, err)

	err = mr.CreateAllIntents(target)
	require.Error(t, err, "a dump containing a smuggled admin.system.users file should be rejected")
	assert.ErrorContains(t, err, "system.buckets.$admin.system.users")
}

// TestNulByteInCollectionNameIsRejected covers the other name that no dump can
// legitimately contain.
func TestNulByteInCollectionNameIsRejected(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "vendordata/c%001.bson")

	mr := newRestoreWithNamespaces(t, "", nil)
	target, err := newActualPath(root)
	require.NoError(t, err)

	err = mr.CreateAllIntents(target)
	require.Error(t, err, "a collection name containing a NUL byte should be rejected")
}

// TestSmuggledUsersFileUnderAdminDBScope covers the remaining scope in which a
// "$admin.system.users" file is honored rather than skipped. mongodump writes
// that name only into a non-admin database's directory; a real admin dump holds
// a plain "system.users.bson". So a $-prefixed file inside admin/ is fabricated,
// and `--db admin` restores it because ShouldRestoreUsersAndRoles is true for
// the admin database even without --restoreDbUsersAndRoles.
func TestSmuggledUsersFileUnderAdminDBScope(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "admin/$admin.system.users.bson")

	mr := newRestoreWithNamespaces(t, "admin", nil)
	target, err := newActualPath(filepath.Join(root, "admin"))
	require.NoError(t, err)

	err = mr.CreateIntentsForDB("admin", target)
	require.Error(t, err, "a $-prefixed users file in a dump of admin should be rejected")
	assert.Nil(
		t,
		mr.manager.Users(),
		"a $-prefixed users file in admin/ should not become the users intent",
	)
}

// TestLegacyOplogMainIsRestorable guards the one collection name mongorestore
// legitimately accepts with an interior "$": the pre-2.8 oplog. Intent.IsOplog
// matches "local.oplog.$main", and mongorestore.go reports a conflict between
// "local/oplog.rs.bson" and "local/oplog.$main.bson", so such a dump must still
// be restorable.
func TestLegacyOplogMainIsRestorable(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "local/oplog.$main.bson")

	mr := newRestoreWithNamespaces(t, "", nil)
	// The manager only routes local.oplog.$main to the oplog intent when
	// mongorestore has asked it to, which it does for --oplogReplay.
	mr.InputOptions.OplogReplay = true
	mr.manager.SetSmartPickOplog(true)

	target, err := newActualPath(root)
	require.NoError(t, err)

	require.NoError(
		t,
		mr.CreateAllIntents(target),
		"a legacy local/oplog.$main.bson dump should still be restorable",
	)
	assert.NotNil(t, mr.manager.Oplog(), "the oplog intent should be created")
}

// TestTruncatedNameMetadataIsValidated covers the branch in getInfoFromFile
// that takes the collection name out of the .metadata.json file when the file
// name is 238 characters and contains "%24". That name is more attacker-
// controlled than the file name, so it must face the same validation.
func TestTruncatedNameMetadataIsValidated(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// getInfoFromFile only consults the metadata file for a name of exactly
	// this length that contains an escaped "$".
	truncatedName := "%24" + strings.Repeat("a", 235)
	require.Len(t, truncatedName, 238, "the file name must hit the truncated-name branch")

	root := t.TempDir()
	dbDir := filepath.Join(root, "vendordata")
	require.NoError(t, os.MkdirAll(dbDir, 0755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dbDir, truncatedName+".bson"), []byte{}, 0644),
	)
	require.NoError(t, os.WriteFile(
		filepath.Join(dbDir, truncatedName+".metadata.json"),
		[]byte(`{"collectionName":"system.buckets.$admin.system.users","indexes":[]}`),
		0644,
	))

	mr := newRestoreWithNamespaces(t, "", []string{"admin.*"})
	target, err := newActualPath(root)
	require.NoError(t, err)

	err = mr.CreateAllIntents(target)
	require.Error(t, err, "a crafted collection name from a metadata file should be rejected")
	assert.Nil(t, mr.manager.Users(), "the smuggled name should not become the users intent")
}

// TestSmuggledUsersFileWithGzip covers the --gzip file naming, which
// getInfoFromFile parses through a separate branch.
func TestSmuggledUsersFileWithGzip(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "vendordata/system.buckets.%24admin.system.users.bson.gz")

	mr := newRestoreWithNamespaces(t, "", []string{"admin.*"})
	mr.InputOptions.Gzip = true

	target, err := newActualPath(root)
	require.NoError(t, err)

	err = mr.CreateAllIntents(target)
	require.Error(t, err, "a smuggled file should be rejected in a gzipped dump too")
	assert.Nil(t, mr.manager.Users(), "the smuggled file should not become the users intent")
}

// TestSmuggledUsersFileWithNamespaceRename checks that a rename cannot be used
// to launder the name: validation must happen before --nsFrom/--nsTo is applied.
func TestSmuggledUsersFileWithNamespaceRename(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "vendordata/system.buckets.$admin.system.users.bson")

	mr := newRestoreWithNamespaces(t, "", nil)
	renamer, err := ns.NewRenamer([]string{"vendordata.*"}, []string{"elsewhere.*"})
	require.NoError(t, err)
	mr.renamer = renamer

	target, err := newActualPath(root)
	require.NoError(t, err)

	err = mr.CreateAllIntents(target)
	require.Error(t, err, "a rename should not launder a smuggled collection name")
	assert.Nil(t, mr.manager.Users(), "the smuggled file should not become the users intent")
}

// TestSmuggledUsersFileAsSingleCollectionTarget covers CreateIntentForCollection,
// the --db/--collection path, which does not share CreateIntentsForDB's checks.
func TestSmuggledUsersFileAsSingleCollectionTarget(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(
		t,
		"vendordata/system.buckets.%24admin.system.users.bson",
		"vendordata/system.buckets.%24admin.system.users.metadata.json",
	)

	mr := newRestoreWithNamespaces(t, "vendordata", nil)
	mr.InputOptions.RestoreDBUsersAndRoles = true

	bsonFile, err := newActualPath(
		filepath.Join(root, "vendordata", "system.buckets.%24admin.system.users.bson"),
	)
	require.NoError(t, err)

	err = mr.CreateIntentForCollection(
		"vendordata",
		"system.buckets.$admin.system.users",
		bsonFile,
	)
	require.Error(t, err, "a smuggled single-collection target should be rejected")
	assert.Nil(t, mr.manager.Users(), "the smuggled file should not become the users intent")
}

// TestOrdinaryAdminDumpIsRestorable guards the legitimate admin dump against
// the db == "admin" rejection: admin holds a plain system.users.bson, ordinary
// user collections, and possibly mongorestore's own leftover staging
// collections. None of those are $-prefixed, so none should be rejected.
func TestOrdinaryAdminDumpIsRestorable(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(
		t,
		"admin/system.users.bson",
		"admin/system.roles.bson",
		"admin/system.version.bson",
		"admin/audit_config.bson",
		"admin/tempusers.bson",
		"admin/temproles.bson",
	)

	mr := newRestoreWithNamespaces(t, "", nil)
	target, err := newActualPath(root)
	require.NoError(t, err)

	require.NoError(t, mr.CreateAllIntents(target), "an ordinary admin dump should be restorable")

	users := mr.manager.Users()
	require.NotNil(t, users, "the users intent should be created")
	assert.Equal(t, "admin", users.DB)
	assert.Equal(t, "system.users", users.C)
}

// archiveExplorer builds a PreludeExplorer over a hand-assembled archive
// prelude holding the given namespaces, which is how an attacker would smuggle
// a crafted collection name through --archive rather than a dump directory.
func archiveExplorer(t *testing.T, namespaces map[string][]string) archive.DirLike {
	t.Helper()

	prelude := &archive.Prelude{
		Header: &archive.Header{
			FormatVersion: "0.1",
			ServerVersion: "8.2.12",
			ToolVersion:   "100.17.0",
		},
		NamespaceMetadatasByDB: map[string][]*archive.CollectionMetadata{},
	}

	for db, collections := range namespaces {
		prelude.DBS = append(prelude.DBS, db)
		for _, collection := range collections {
			meta := &archive.CollectionMetadata{Database: db, Collection: collection}
			prelude.NamespaceMetadatas = append(prelude.NamespaceMetadatas, meta)
			prelude.NamespaceMetadatasByDB[db] = append(prelude.NamespaceMetadatasByDB[db], meta)
		}
	}

	explorer, err := prelude.NewPreludeExplorer()
	require.NoError(t, err)

	return explorer
}

// TestSmuggledUsersNamespaceInArchiveIsRejected covers the --archive form of
// the attack. The prelude names the namespace directly, so no file layout is
// needed; PreludeExplorer synthesizes the same file names CreateIntentsForDB
// parses for a directory dump.
func TestSmuggledUsersNamespaceInArchiveIsRejected(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	testCases := []struct {
		name       string
		db         string
		collection string
	}{
		{
			name:       "timeseries-wrapped users collection",
			db:         "vendordata",
			collection: "system.buckets.$admin.system.users",
		},
		{
			name:       "dollar-prefixed users collection in the admin database",
			db:         "admin",
			collection: "$admin.system.users",
		},
		{
			name:       "unrecognized dollar-prefixed collection",
			db:         "vendordata",
			collection: "$admin.system.somethingelse",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mr := newRestoreWithNamespaces(t, "", []string{"admin.*"})
			mr.InputOptions.Archive = "smuggled.archive"

			target := archiveExplorer(t, map[string][]string{tc.db: {tc.collection}})

			err := mr.CreateAllIntents(target)
			require.Error(t, err, "a crafted namespace in an archive prelude should be rejected")
			assert.Nil(t, mr.manager.Users(), "the smuggled namespace should not become the users intent")
			assert.Nil(t, mr.manager.Roles(), "the smuggled namespace should not become the roles intent")
		})
	}
}

// TestEmptyCollectionNameDoesNotPanic covers a file named ".bson", which
// getInfoFromFile reports as a BSON file with an empty collection name.
func TestEmptyCollectionNameDoesNotPanic(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "vendordata/.bson")

	mr := newRestoreWithNamespaces(t, "", nil)
	target, err := newActualPath(root)
	require.NoError(t, err)

	require.NotPanics(
		t,
		func() { _ = mr.CreateAllIntents(target) },
		"an empty collection name should not panic",
	)
}

// TestDBScopedUsersRestoreStillWorks is the regression guard on the fix: the
// legitimate producer of "$admin.system.*" files is
// `mongodump --db X --dumpDbUsersAndRoles`, and restoring that dump with
// `--db X` must still pick them up as the users and roles intents.
func TestDBScopedUsersRestoreStillWorks(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(
		t,
		"vendordata/$admin.system.users.bson",
		"vendordata/$admin.system.roles.bson",
		"vendordata/$admin.system.version.bson",
		"vendordata/c1.bson",
	)

	mr := newRestoreWithNamespaces(t, "vendordata", nil)
	mr.InputOptions.RestoreDBUsersAndRoles = true

	target, err := newActualPath(filepath.Join(root, "vendordata"))
	require.NoError(t, err)

	require.NoError(
		t,
		mr.CreateIntentsForDB("vendordata", target),
		"a --dumpDbUsersAndRoles dump should still be restorable with --db",
	)

	require.NotNil(t, mr.manager.Users(), "the users intent should be created")
	require.NotNil(t, mr.manager.Roles(), "the roles intent should be created")
	assert.Equal(t, "$admin.system.users", mr.manager.Users().C)
	assert.Equal(t, "$admin.system.roles", mr.manager.Roles().C)
}

// TestSmuggledUsersFileIsNotRestoredUnderDBScope checks that the db-scoped
// restore, which legitimately accepts "$admin.system.*", still rejects the
// timeseries-wrapped form.
func TestSmuggledUsersFileIsNotRestoredUnderDBScope(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	root := writeDumpDir(t, "vendordata/system.buckets.$admin.system.users.bson")

	mr := newRestoreWithNamespaces(t, "vendordata", nil)
	target, err := newActualPath(filepath.Join(root, "vendordata"))
	require.NoError(t, err)

	err = mr.CreateIntentsForDB("vendordata", target)
	require.Error(t, err, "the smuggled file should be rejected under --db as well")
	assert.Nil(t, mr.manager.Users(), "smuggled file should not become the users intent")
}
