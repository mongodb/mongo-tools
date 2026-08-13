// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongotop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/internal/testcmd"
	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// mongotop has no in-process entry point: main() does its own option validation
// and calls os.Exit, so these tests build the tool once and run it as a
// subprocess, reading its exit status and output.

// TestMongotopJSON checks that --json output is one JSON object per row, each
// holding the map of namespace totals.
//
// mongotop needs two polls of the top command before it can report a diff, so
// --rowcount n prints n rows after n+1 polls.
func TestMongotopJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	for _, rowCount := range []int{1, 5} {
		t.Run(fmt.Sprintf("rowcount=%d", rowCount), func(t *testing.T) {
			stdout, stderr, err := runMongotop(t, "--json", "--rowcount", strconv.Itoa(rowCount))
			require.NoError(t, err, "mongotop exits successfully: %s", stderr)

			rows := testcmd.Rows(stdout)
			require.Len(t, rows, rowCount, "--rowcount is the number of rows printed")
			for i, row := range rows {
				diff := parseTopDiff(t, row)
				assert.NotEmpty(t, diff.Totals,
					"row %d reports totals for at least one namespace", i)
			}
		})
	}
}

// TestMongotopReportsActivity checks that the reported per-namespace counts
// follow the activity on the server: a collection being read from and written to
// is reported with nonzero read and write counts, an idle collection with
// nothing at all.
//
// The reported times are not asserted on, because the operations here are cheap
// enough that the sampled time is routinely 0. Neither is per-metric
// exclusivity: the driver reads in order to find out where to send a write, so a
// write-only workload is not something a Go test can produce. Counts are only
// ever compared against zero, for the same reason — the exact values are not
// reproducible.
func TestMongotopReportsActivity(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongotop_reports_test"
		busyName = "busy"
		idleName = "idle"
		busyNS   = dbName + "." + busyName
		idleNS   = dbName + "." + idleName
		rowCount = 8
	)

	client, err := testutil.GetBareSession(t)
	require.NoError(t, err, "can connect to the test server")
	t.Cleanup(func() {
		assert.NoError(
			t,
			client.Database(dbName).Drop(context.Background()),
			"the test database is dropped",
		)
		assert.NoError(t, client.Disconnect(context.Background()), "the client disconnects")
	})
	require.NoError(t, client.Database(dbName).Drop(t.Context()), "any earlier data is dropped")

	// The idle collection has to exist before mongotop starts, or it will not be
	// in the totals at all, and its one write must land before the first poll.
	_, err = client.Database(dbName).Collection(idleName).InsertOne(t.Context(), bson.D{{"x", 1}})
	require.NoError(t, err, "the idle collection is created")

	stopWorkload := workUntilStopped(t, client.Database(dbName).Collection(busyName))

	stdout, stderr, err := runMongotop(t, "--json", "--rowcount", strconv.Itoa(rowCount))
	stopWorkload()
	require.NoError(t, err, "mongotop exits successfully: %s", stderr)

	// Only the sum over every row is guaranteed nonzero: any single row covers a
	// one-second window that the workload may have spent between operations. The
	// busy collection is not in the earliest rows at all, since it does not exist
	// until the workload's first insert creates it.
	var busyReads, busyWrites, idleOps int
	var sawBusy bool
	for _, row := range testcmd.Rows(stdout) {
		diff := parseTopDiff(t, row)
		require.Contains(t, diff.Totals, idleNS, "the idle namespace is reported")
		idleOps += diff.Totals[idleNS].Total.Count

		busy, ok := diff.Totals[busyNS]
		if !ok {
			continue
		}
		sawBusy = true
		busyReads += busy.Read.Count
		busyWrites += busy.Write.Count

		// The reported total is the read and write activity added up, give or take
		// the one operation that can land between the two being sampled.
		assert.InDelta(t, busy.Read.Count+busy.Write.Count, busy.Total.Count, 1,
			"the total count for %#q is the read and write counts it is made of", busyNS)
		assert.GreaterOrEqual(t, busy.Total.Time, busy.Read.Time+busy.Write.Time,
			"the total time for %#q is at least the read and write time it is made of", busyNS)
	}
	require.True(t, sawBusy, "the busy namespace is reported at all")
	assert.Positive(t, busyReads, "the collection being read from reports reads")
	assert.Positive(t, busyWrites, "the collection being written to reports writes")
	assert.Zero(t, idleOps, "the idle collection reports no activity")
}

// TestMongotopRejectsInvalidOptions checks the ways of invoking mongotop that it
// has to reject, and that --version and --help are not among them.
//
// An empty --port is not among them: it falls back to the driver's default of
// 27017, so whether it fails is a fact about the machine rather than about
// mongotop.
func TestMongotopRejectsInvalidOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	// Each case carries the whole argument list, including the --rowcount that
	// keeps a case which does get as far as running from running forever: a
	// --rowcount appended afterwards would override the ones under test.
	for _, c := range []struct {
		name        string
		args        []string
		errContains string
	}{
		{
			name:        "a negative rowcount",
			args:        []string{"--rowcount", "-2"},
			errContains: "invalid value for --rowcount: -2",
		},
		{
			name: "a rowcount that is not a number",
			args: []string{"--rowcount", "hello"},
			// The flag parser writes the option prefix its platform uses, so
			// this matches from the long name on rather than from `-n`: on
			// Windows the same message reads "flag `/n, /rowcount'".
			errContains: "rowcount' (expected int)",
		},
		{
			name:        "a negative sleep time",
			args:        []string{"-4", "--rowcount", "1"},
			errContains: "unknown option `4`",
		},
		{
			name:        "a sleep time that is not a number",
			args:        []string{"forever", "--rowcount", "1"},
			errContains: "invalid sleep time: forever",
		},
		{
			name:        "more than one positional argument",
			args:        []string{"2", "3", "--rowcount", "1"},
			errContains: "provide only one polling interval in seconds",
		},
		{
			name:        "an unknown option",
			args:        []string{"--elder", "--rowcount", "1"},
			errContains: "unknown option `elder`",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, err := runMongotop(t, c.args...)
			testcmd.RequireExitFailure(t, err, "mongotop")
			assert.Contains(t, stderr, c.errContains, "%s is rejected", c.name)
		})
	}

	t.Run("a port that is not a number", func(t *testing.T) {
		// The server is named here rather than by GetBareArgs, which supplies a
		// connection string when TOOLS_TESTING_MONGOD is set: a --port alongside a
		// URI is rejected for conflicting with it, before its value is looked at.
		_, stderr, err := testcmd.Run(t, mongotopBinary(t), "--port", "hello", "--rowcount", "1")
		testcmd.RequireExitFailure(t, err, "mongotop")
		assert.Contains(t, stderr, "port must be an integer", "a non-numeric port is rejected")
	})

	t.Run("a server that cannot be reached is an error", func(t *testing.T) {
		// Without a shortened selection timeout this case spends the driver's
		// default 30 seconds waiting for a server that is never coming.
		_, stderr, err := testcmd.Run(t, mongotopBinary(t), slices.Concat(
			testopts.GetSSLArgs(), testopts.GetAuthArgs(),
			[]string{
				"--host", "localhost", "--port", testcmd.UnreachablePort(t),
				"--serverSelectionTimeout", "2",
				"--rowcount", "1",
			},
		)...)
		testcmd.RequireExitFailure(t, err, "mongotop")
		assert.Contains(t, stderr, "error connecting to host",
			"the failure is the connection, not something else")
	})

	for _, flag := range []string{"--version", "--help"} {
		t.Run(flag+" succeeds", func(t *testing.T) {
			stdout, stderr, err := runMongotop(t, flag)
			require.NoError(t, err, "mongotop exits successfully: %s", stderr)
			assert.Contains(t, stdout, "mongotop", "the tool names itself")
		})
	}
}

// TestMongotopAuth checks that mongotop authenticates with the credentials it is
// given, and fails when the password is wrong rather than reporting on the
// server's namespaces anyway.
func TestMongotopAuth(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	// The credentials are supplied here rather than by GetBareArgs, which would
	// supply the right password for both cases.
	auth := testopts.GetAuthOptions()
	args := slices.Concat(testopts.GetSSLArgs(), []string{
		"--host", testcmd.ServerHostPort(t),
		"--authenticationDatabase", auth.Source,
		"--username", auth.Username,
		"--serverSelectionTimeout", "5",
		"--json", "--rowcount", "1",
	})

	t.Run("the right password succeeds", func(t *testing.T) {
		stdout, stderr, err := testcmd.Run(t, mongotopBinary(t),
			slices.Concat(args, []string{"--password", auth.Password})...)
		require.NoError(t, err, "mongotop exits successfully: %s", stderr)

		rows := testcmd.Rows(stdout)
		require.Len(t, rows, 1, "one row is printed")
		assert.NotEmpty(t, parseTopDiff(t, rows[0]).Totals, "the row reports totals")
	})

	t.Run("a wrong password fails", func(t *testing.T) {
		stdout, stderr, err := testcmd.Run(t, mongotopBinary(t),
			slices.Concat(args, []string{"--password", "not-the-password"})...)
		testcmd.RequireExitFailure(t, err, "mongotop")
		assert.Contains(t, stderr, "Authentication failed",
			"the failure is the authentication, not something else")
		assert.Empty(t, testcmd.Rows(stdout), "no rows are printed")
	})
}

// TestMongotopRejectsMongos checks that mongotop refuses to run against a
// mongos, which does not support the top command, rather than failing later with
// whatever the mongos makes of it.
func TestMongotopRejectsMongos(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)

	const wantError = "cannot run mongotop against a mongos"

	for _, c := range []struct {
		name string
		args []string
	}{
		{"with no arguments", nil},
		{"with a sleep time", []string{"2"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, err := runMongotop(t, append(slices.Clone(c.args), "--rowcount", "1")...)
			testcmd.RequireExitFailure(t, err, "mongotop")
			assert.Contains(t, stderr, wantError, "the failure names the mongos")
		})
	}
}

// workUntilStopped reads from and writes to the collection until the returned
// function is called, which it is safe to call more than once.
func workUntilStopped(t *testing.T, coll *mongo.Collection) func() {
	t.Helper()

	done := make(chan struct{})
	var stopped sync.Once
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			ctx := context.Background()
			if _, err := coll.InsertOne(ctx, bson.D{{"x", i}}); err != nil {
				return
			}
			if err := coll.FindOne(ctx, bson.D{{"x", i}}).Err(); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	})

	stop := func() {
		stopped.Do(func() {
			close(done)
			wg.Wait()
		})
	}
	t.Cleanup(stop)
	return stop
}

// parseTopDiff decodes one row of --json output.
func parseTopDiff(t *testing.T, row string) TopDiff {
	t.Helper()
	var diff TopDiff
	require.NoError(t, json.Unmarshal([]byte(row), &diff), "the row is JSON: %s", row)
	return diff
}

// runMongotop runs mongotop against the test server. Rows go to stdout and
// everything else to stderr, so they are returned separately rather than
// merged: a diagnostic line would otherwise be read as a row.
func runMongotop(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return testcmd.RunAgainstTestServer(t, mongotopBinary(t), args...)
}

// buildMongotop builds the tool once for the whole package. Building rather
// than `go run` means a build failure cannot be mistaken for the tool exiting
// nonzero. The binary needs the platform's extension, because Windows cannot
// exec a path without one.
var buildMongotop = sync.OnceValues(func() (string, error) {
	binary := filepath.Join(os.TempDir(), "mongotop-integration-test"+platform.GetLocalBinaryExt())
	if out, err := exec.Command("go", "build", "-o", binary, "./main").
		CombinedOutput(); err != nil {
		return "", fmt.Errorf("building mongotop: %v: %s", err, out)
	}
	return binary, nil
})

func mongotopBinary(t *testing.T) string {
	t.Helper()
	binary, err := buildMongotop()
	require.NoError(t, err, "mongotop builds")
	return binary
}
