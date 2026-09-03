// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package testutil implements functions for filtering and configuring tests.
package testutil

import (
	"context"
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
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// GetBareSession returns a client from the environment or from a default host
// and port. The underlying session provider is closed when the test ends, so
// the returned client must not be used after that.
func GetBareSession(t *testing.T) (*mongo.Client, error) {
	t.Helper()

	sessionProvider, _, err := GetBareSessionProvider(t)
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

	toolOptions, err := testopts.GetToolOptions()
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
