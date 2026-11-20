// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package testutil implements functions for filtering and configuring tests.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/db/dsctest"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// GetBareSession returns a client from the environment or from a default host
// and port. The underlying session provider is closed when the test ends, so
// the returned client must not be used after that.
func GetBareSession(t *testing.T) (*mongo.Client, error) {
	return GetBareSessionForURI(t, os.Getenv(testopts.URIEnvVar))
}

// GetBareSessionForURI is GetBareSession for a specific connection string.
func GetBareSessionForURI(t *testing.T, uri string) (*mongo.Client, error) {
	t.Helper()

	sessionProvider, _, err := GetBareSessionProviderForURI(t, uri)
	if err != nil {
		return nil, err
	}
	session, err := sessionProvider.GetSession()
	if err != nil {
		return nil, err
	}
	return session, nil
}

// GetBareSessionProvider returns a session provider from the environment or
// from a default host and port. The provider is closed when the test ends, so
// the caller must not close it, and must not use it after that.
//
// An unclosed provider leaves the driver's topology-monitoring goroutines
// running for the remainder of the test binary's run, which is why this takes a
// *testing.T rather than leaving the close to each caller.
func GetBareSessionProvider(
	t *testing.T,
) (*db.SessionProvider, *options.ToolOptions, error) {
	t.Helper()

	return GetBareSessionProviderForURI(t, os.Getenv(testopts.URIEnvVar))
}

// GetBareSessionProviderForURI is GetBareSessionProvider for a specific connection string.
func GetBareSessionProviderForURI(
	t *testing.T,
	uri string,
) (*db.SessionProvider, *options.ToolOptions, error) {
	t.Helper()

	toolOptions, err := testopts.GetToolOptionsForURI(uri)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"error getting tool options to create a bare session provider: %w",
			err,
		)
	}

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(sessionProvider.Close)

	return sessionProvider, toolOptions, nil
}

// oplogReplayRoleName is the role EnsureOplogReplayUser grants its user: the documentation's
// prescribed minimum for `mongorestore --oplogReplay`, anyAction on anyResource, and nothing
// else.
const oplogReplayRoleName = "oplogReplayAll"

// EnsureBackupUser creates the user that mongodump runs authenticate as
// (testopts.BackupUsername) when it does not already exist, holding exactly the backup role the
// documentation prescribes for mongodump. The cluster script creates only the bootstrap user,
// because the runner cannot create a second user itself, so the first test that needs this user
// creates it here.
func EnsureBackupUser(t *testing.T) {
	ensureUser(t, testopts.BackupUsername, createBackupUser)
}

// EnsureRestoreUser creates the user that mongorestore runs authenticate as
// (testopts.RestoreUsername) when it does not already exist, holding exactly the restore role
// the documentation prescribes for mongorestore. Like EnsureBackupUser, it is a no-op without
// the auth test type.
func EnsureRestoreUser(t *testing.T) {
	ensureUser(t, testopts.RestoreUsername, createRestoreUser)
}

// EnsureOplogReplayUser creates the user that tests running `mongorestore --oplogReplay`
// authenticate as (testopts.OplogReplayUsername), holding the anyAction-on-anyResource role the
// documentation prescribes for oplog replay. Like EnsureBackupUser, it is a no-op without the
// auth test type.
func EnsureOplogReplayUser(t *testing.T) {
	ensureUser(t, testopts.OplogReplayUsername, createOplogReplayUser)
}

// EnsureSharedUser creates the shared test user that auth names, when it names one, so that a
// helper handing auth to a tool invocation does not have to know which shared identity it is.
// Auth carrying a per-test user's credentials is left alone: those tests create their own users.
func EnsureSharedUser(t *testing.T, auth options.Auth) {
	t.Helper()

	switch auth.Username {
	case testopts.BackupUsername:
		EnsureBackupUser(t)
	case testopts.RestoreUsername:
		EnsureRestoreUser(t)
	case testopts.OplogReplayUsername:
		EnsureOplogReplayUser(t)
	}
}

// DropAdminSystemViews removes admin.system.views, which restores of the timeseries fixtures
// leave behind on a shared cluster. The backup and restore roles lack find on it — the backup
// role grants find on admin's system.users, system.roles, system.js, and system.profile
// collections, but not on system.views — so its presence makes a dump of the admin database or
// of everything fail as the tool user. Tests that dump admin or the whole cluster call this to
// state their precondition that it is not there. It is a no-op without the auth test type:
// without authentication every connection reads as the tool user's better.
func DropAdminSystemViews(t *testing.T) {
	t.Helper()

	if !testtype.HasTestType(testtype.AuthTestType) {
		return
	}

	session, err := GetBareSession(t)
	require.NoError(t, err, "connecting as the harness user to drop admin.system.views")

	err = session.Database("admin").Collection("system.views").Drop(t.Context())
	// A mongos refuses to drop any collection in the admin database, so the precondition cannot
	// be established on a sharded cluster. That is tolerable: the sharded CI variants run the
	// packages in an order where nothing has left admin.system.views behind by the time the
	// dumps under test run.
	var cmdErr mongo.CommandError
	if err != nil &&
		!(errors.As(err, &cmdErr) && cmdErr.Name == "IllegalOperation") {
		require.NoError(t, err, "dropping admin.system.views")
	}
}

// ensureUser creates one of the users the tests share when it does not already exist. Existence
// is re-checked on every call rather than cached: a test that restores admin's users with --drop
// (TestDumpRestorePreservesAdminUsersAndRoles) can remove a shared user mid-run, and a cached
// "already created" would leave the later tests failing authentication with no hint why.
func ensureUser(t *testing.T, username string, create func(*mongo.Client) error) {
	t.Helper()

	if !testtype.HasTestType(testtype.AuthTestType) {
		return
	}

	if err := createUserIfMissing(t, username, create); err != nil {
		t.Fatalf("could not create the %s user: %v", username, err)
	}
}

func createUserIfMissing(t *testing.T, username string, create func(*mongo.Client) error) error {
	session, err := GetBareSession(t)
	if err != nil {
		return fmt.Errorf("connecting as the harness user: %w", err)
	}

	ctx := context.Background()
	adminDB := session.Database("admin")

	var info struct {
		Users []bson.D `bson:"users"`
	}
	err = adminDB.RunCommand(ctx, bson.D{
		{Key: "usersInfo", Value: bson.A{bson.D{
			{Key: "user", Value: username},
			{Key: "db", Value: "admin"},
		}}},
	}).Decode(&info)
	if err != nil {
		return fmt.Errorf("looking up the %s user: %w", username, err)
	}
	if len(info.Users) > 0 {
		return nil
	}

	err = create(session)
	// A concurrently running test binary can win the race to create the user between the lookup
	// above and the create callback. A failed ensure never retries, so that outcome is success
	// too.
	var cmdErr mongo.CommandError
	if err != nil && errors.As(err, &cmdErr) && cmdErr.Name == "UserAlreadyExists" {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creating the %s user: %w", username, err)
	}

	return nil
}

func createBackupUser(session *mongo.Client) error {
	ctx := context.Background()
	return session.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: testopts.BackupUsername},
		{Key: "pwd", Value: testopts.BackupPassword},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "backup"}, {Key: "db", Value: "admin"}},
		}},
	}).Err()
}

func createRestoreUser(session *mongo.Client) error {
	ctx := context.Background()
	return session.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: testopts.RestoreUsername},
		{Key: "pwd", Value: testopts.RestorePassword},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "restore"}, {Key: "db", Value: "admin"}},
		}},
	}).Err()
}

func createOplogReplayUser(session *mongo.Client) error {
	ctx := context.Background()
	adminDB := session.Database("admin")

	// The role is created before the user that holds it, and tolerates already existing: the
	// user-existence check in createUserIfMissing ran over a connection of its own, so a
	// concurrently running test binary can create either in between.
	err := adminDB.RunCommand(ctx, bson.D{
		{Key: "createRole", Value: oplogReplayRoleName},
		{Key: "privileges", Value: bson.A{bson.D{
			{Key: "resource", Value: bson.D{{Key: "anyResource", Value: true}}},
			{Key: "actions", Value: bson.A{"anyAction"}},
		}}},
		{Key: "roles", Value: bson.A{}},
	}).Err()
	var cmdErr mongo.CommandError
	if err != nil && !(errors.As(err, &cmdErr) && cmdErr.Name == "RoleAlreadyExists") {
		return fmt.Errorf("creating the oplog-replay role: %w", err)
	}

	return adminDB.RunCommand(ctx, bson.D{
		{Key: "createUser", Value: testopts.OplogReplayUsername},
		{Key: "pwd", Value: testopts.OplogReplayPassword},
		{Key: "roles", Value: bson.A{bson.D{
			{Key: "role", Value: oplogReplayRoleName},
			{Key: "db", Value: "admin"},
		}}},
	}).Err()
}

// GetFCV returns the featureCompatibilityVersion string for an mgo Session
// or the empty string if it can't be found.
func GetFCV(s *mongo.Client) string {
	coll := s.Database("admin").Collection("system.version")
	var result struct {
		Version string
	}
	res := coll.FindOne(context.TODO(), bson.M{"_id": "featureCompatibilityVersion"})
	//nolint:errcheck
	res.Decode(&result)
	return result.Version
}

// CompareFCV compares two strings as dot-delimited tuples of integers.
func CompareFCV(x, y string) (int, error) {
	left, err := dottedStringToSlice(x)
	if err != nil {
		return 0, err
	}
	right, err := dottedStringToSlice(y)
	if err != nil {
		return 0, err
	}

	// Ensure left is the shorter one, flip logic if necessary.
	inverter := 1
	if len(right) < len(left) {
		inverter = -1
		left, right = right, left
	}

	for i := range left {
		switch {
		case left[i] < right[i]:
			return -1 * inverter, nil
		case left[i] > right[i]:
			return 1 * inverter, nil
		}
	}

	// compared equal to length of left. If right is longer, then left is less
	// than right (-1) (modulo the inverter)
	if len(left) < len(right) {
		return -1 * inverter, nil
	}

	return 0, nil
}

func SkipIfFCVLessThan(t *testing.T, versionStr string, reason string) {
	session, err := GetBareSession(t)
	require.NoError(t, err)

	fcv := GetFCV(session)
	if cmp, err := CompareFCV(fcv, versionStr); err != nil || cmp < 0 {
		if err != nil {
			t.Errorf("error getting FCV: %v", err)
		}
		t.Skipf(
			"Skipping test because %s. Requires server with FCV 6.0 or later; found %v",
			reason,
			fcv,
		)
	}
}

// SkipUnlessStandalone skips the test unless it is connected to a standalone
// server. Some operations (e.g. writing local.oplog.rs as a collection) are
// only permitted on a standalone: a replica set rejects direct oplog writes and
// mongos rejects writes to the local database.
func SkipUnlessStandalone(t *testing.T) {
	sessionProvider, _, err := GetBareSessionProvider(t)
	require.NoError(t, err)

	nodeType, err := sessionProvider.GetNodeType()
	require.NoError(t, err)
	if nodeType != db.Standalone {
		t.Skipf("Skipping test because it requires a standalone server, not %q", nodeType)
	}
}

// SkipUnlessReplicaSet skips the test unless it is connected to a replica set.
// Some operations (e.g. reading the live replication oplog) only exist on a
// replica set member, and mongos and standalone servers reject them. Writing
// local.oplog.rs as a collection is a standalone-only operation, for which use
// SkipUnlessStandalone instead.
func SkipUnlessReplicaSet(t *testing.T) {
	t.Helper()

	sessionProvider, _, err := GetBareSessionProvider(t)
	require.NoError(t, err)

	isReplicaSet, err := sessionProvider.IsReplicaSet()
	require.NoError(t, err)
	if !isReplicaSet {
		t.Skip("Skipping test because it requires a replica set")
	}
}

// SkipIfSharded skips the test when it is connected to a sharded cluster (via
// mongos). Integration tests run against sharded clusters by default, so use
// this for the ones that depend on behavior a sharded cluster does not support,
// naming that behavior in the reason.
func SkipIfSharded(t *testing.T, reason string) {
	t.Helper()

	sessionProvider, _, err := GetBareSessionProvider(t)
	require.NoError(t, err)

	nodeType, err := sessionProvider.GetNodeType()
	require.NoError(t, err)
	if nodeType == db.Mongos {
		t.Skipf("Skipping test on a sharded cluster because %s", reason)
	}
}

func dottedStringToSlice(s string) ([]int, error) {
	parts := make([]int, 0, 2)
	for v := range strings.SplitSeq(s, ".") {
		i, err := strconv.Atoi(v)
		if err != nil {
			return parts, err
		}
		parts = append(parts, i)
	}
	return parts, nil
}

// MergeOplogStreams combines oplog arrays such that the order of entries is
// random, but order-preserving with respect to each initial stream.
func MergeOplogStreams(input [][]db.Oplog) []db.Oplog {
	// Copy input op arrays so we can destructively shuffle them together
	streams := make([][]db.Oplog, len(input))
	opCount := 0
	for i, v := range input {
		streams[i] = make([]db.Oplog, len(v))
		copy(streams[i], v)
		opCount += len(v)
	}

	ops := make([]db.Oplog, 0, opCount)
	for len(streams) != 0 {
		// randomly pick a stream to add an op
		rand.Shuffle(len(streams), func(i, j int) {
			streams[i], streams[j] = streams[j], streams[i]
		})
		ops = append(ops, streams[0][0])
		// remove the op and its stream if empty
		streams[0] = streams[0][1:]
		if len(streams[0]) == 0 {
			streams = streams[1:]
		}
	}

	return ops
}

// MakeTempDir will attempt to create a temp directory. If it fails it will
// abort the test. It returns two values. The first is the string containing
// the path to the temp directory. The second is a cleanup func that will
// remove the temp directory. You should always call the cleanup func with
// `defer` immedatiately after calling this function:
//
//	dir, cleanup := testutil.MakeTempDir(t)
//	defer cleanup()
//
// If the `TOOLS_TESTING_NO_CLEANUP` env var is not empty, then the cleanup
// function will not delete the directory. This can be useful when
// investigating test failures.
func MakeTempDir(t *testing.T) (string, func()) {
	require := require.New(t)

	dir, err := os.MkdirTemp("", "mongo-tools-test")
	require.NoError(err, "can create temp directory")
	cleanup := func() {
		if os.Getenv("TOOLS_TESTING_NO_CLEANUP") == "" {
			err = os.RemoveAll(dir)
			if err != nil {
				t.Fatalf("Failed to delete temp directory: %v", err)
			}
		}
	}
	return dir, cleanup
}

var atlasDomains = []string{
	".mongo.com",
	".mongodb.net",
	".mongodb-qa.net",
	".mongodb-dev.net",
	".mmscloudteam.com",
	".mmscloudtest.com",
	".mongodbgov.net",
	".mongodbgov-local.net",
	".mongodbgov-dev.net",
	".mongodbgov-qa.net",
}

// SkipForAtlasCluster will skip the test if `TOOLS_TESTING_MONGOD` is an Atlas URI.
func SkipForAtlasCluster(t *testing.T, reason string) {
	uri := os.Getenv(testopts.URIEnvVar)
	if uri == "" {
		return
	}

	for _, d := range atlasDomains {
		if strings.Contains(uri, d) {
			t.Skipf(
				"The %#q env var is for an Atlas cluster: %s",
				testopts.URIEnvVar,
				reason,
			)
		}
	}
}

// SkipForDisaggregatedStorage will skip the test if the server is running with disaggregated
// storage (DSC) enabled.
//
// Use this for tests that depend on a server feature DSC does not support, naming that feature in
// the reason. It deliberately keys off DSC rather than off the unsupported operation failing, so an
// unexpected failure of that same operation elsewhere still fails the test loudly instead of
// quietly skipping it.
func SkipForDisaggregatedStorage(t *testing.T, reason string) {
	t.Helper()

	session, err := GetBareSession(t)
	require.NoError(t, err, "getting a session to check for disaggregated storage")

	dsctest.SkipForDisaggregatedStorage(t, session, reason)
}

// SkipUnlessDisaggregatedStorage will skip the test if the server is not running with disaggregated
// storage (DSC) enabled.
//
// Use this for tests that exercise a guardrail that only fires on a DSC cluster, so they only run
// against the DSC CI variant.
func SkipUnlessDisaggregatedStorage(t *testing.T, reason string) {
	session, err := GetBareSession(t)
	require.NoError(t, err, "getting a session to check for disaggregated storage")

	dsctest.SkipUnlessDisaggregatedStorage(t, session, reason)
}

// WriteTempFile writes the contents of r to a tempfile, then hands you back the (closed) file.
func WriteTempFile(t *testing.T, r io.Reader) *os.File {
	file, err := os.CreateTemp("", "toolstest_")
	require.NoError(t, err)

	data, err := io.ReadAll(r)
	require.NoError(t, err)

	n, err := file.Write(data)
	require.NoError(t, err, "write file")
	require.Equal(t, len(data), n, "write all data to file")
	require.NoError(t, file.Close())

	return file
}
