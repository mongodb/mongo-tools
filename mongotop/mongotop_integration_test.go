// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongotop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// mongotop has no in-process entry point: main() does its own option validation
// and calls os.Exit, so these tests build the tool once and run it the way the
// JS tests they replace did, reading its exit status and output.

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

			rows := topRows(stdout)
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
// The JS test also asserted on the reported times, and that a read-only or
// write-only workload left the other metric at zero. Neither is carried over:
// the operations here are cheap enough that the sampled time is routinely 0,
// which is what made the JS test flaky, and the driver reads to find out where
// to send a write, so a write-only workload is not something a Go test can
// produce. Counts are only ever compared against zero, for the same reason —
// the exact values are not reproducible.
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

	client, err := testutil.GetBareSession()
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
	for _, row := range topRows(stdout) {
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
// The JS test also passed an empty --port, expecting a failure. That is not
// carried over: an empty port falls back to the driver's default of 27017, so
// the failure is only that nothing happens to be listening there, which is a
// fact about the machine rather than about mongotop.
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
			name:        "a rowcount that is not a number",
			args:        []string{"--rowcount", "hello"},
			errContains: "invalid argument for flag `-n, --rowcount'",
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
			assert.Error(t, err, "mongotop exits nonzero")
			assert.Contains(t, stderr, c.errContains, "%s is rejected", c.name)
		})
	}

	t.Run("a port that is not a number", func(t *testing.T) {
		// The server is named here rather than by GetBareArgs, which supplies a
		// connection string when TOOLS_TESTING_MONGOD is set: a --port alongside a
		// URI is rejected for conflicting with it, before its value is looked at.
		_, stderr, err := runMongotopArgs(t, "--port", "hello", "--rowcount", "1")
		assert.Error(t, err, "mongotop exits nonzero")
		assert.Contains(t, stderr, "port must be an integer", "a non-numeric port is rejected")
	})

	t.Run("a server that cannot be reached is an error", func(t *testing.T) {
		// Without a shortened selection timeout this case spends the driver's
		// default 30 seconds waiting for a server that is never coming.
		_, stderr, err := runMongotopArgs(t, slices.Concat(
			testutil.GetSSLArgs(), testutil.GetAuthArgs(),
			[]string{
				"--host", "localhost", "--port", unreachablePort(t),
				"--serverSelectionTimeout", "2",
				"--rowcount", "1",
			},
		)...)
		assert.Error(t, err, "mongotop exits nonzero")
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
// server's namespaces anyway. The JS tests covered this by running their whole
// body a second time under an auth passthrough.
func TestMongotopAuth(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	// The credentials are supplied here rather than by GetBareArgs, which would
	// supply the right password for both cases.
	auth := testutil.GetAuthOptions()
	args := slices.Concat(testutil.GetSSLArgs(), []string{
		"--host", testServerHostPort(t),
		"--authenticationDatabase", auth.Source,
		"--username", auth.Username,
		"--serverSelectionTimeout", "5",
		"--json", "--rowcount", "1",
	})

	t.Run("the right password succeeds", func(t *testing.T) {
		stdout, stderr, err := runMongotopArgs(t,
			slices.Concat(args, []string{"--password", auth.Password})...)
		require.NoError(t, err, "mongotop exits successfully: %s", stderr)

		rows := topRows(stdout)
		require.Len(t, rows, 1, "one row is printed")
		assert.NotEmpty(t, parseTopDiff(t, rows[0]).Totals, "the row reports totals")
	})

	t.Run("a wrong password fails", func(t *testing.T) {
		stdout, stderr, err := runMongotopArgs(t,
			slices.Concat(args, []string{"--password", "not-the-password"})...)
		assert.Error(t, err, "mongotop exits nonzero")
		assert.Contains(t, stderr, "Authentication failed",
			"the failure is the authentication, not something else")
		assert.Empty(t, topRows(stdout), "no rows are printed")
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
			assert.Error(t, err, "mongotop exits nonzero")
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

// topRows returns the non-empty lines of mongotop's output.
func topRows(stdout string) []string {
	var rows []string
	for line := range strings.SplitSeq(stdout, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			rows = append(rows, trimmed)
		}
	}
	return rows
}

// runMongotop runs mongotop against the test server. Rows go to stdout and
// everything else to stderr, so they are returned separately rather than
// merged: a diagnostic line would otherwise be read as a row.
func runMongotop(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runMongotopArgs(t, append(testutil.GetBareArgs(), args...)...)
}

func runMongotopArgs(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(mongotopBinary(t), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// buildMongotop builds the tool once for the whole package. Building rather
// than `go run` means a build failure cannot be mistaken for the tool exiting
// nonzero. The binary needs the platform's extension, because Windows cannot
// exec a path without one.
var buildMongotop = sync.OnceValues(func() (string, error) {
	binary := filepath.Join(os.TempDir(), "mongotop-integration-test"+platform.GetLocalBinaryExt())
	if out, err := exec.Command("go", "build", "-o", binary, "./main").CombinedOutput(); err != nil {
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

// unreachablePort is a port the test server is not listening on.
func unreachablePort(t *testing.T) string {
	t.Helper()
	_, port, err := net.SplitHostPort(testServerHostPort(t))
	require.NoError(t, err, "the test server address has a port")
	number, err := strconv.Atoi(port)
	require.NoError(t, err, "the test server port is a number")
	return strconv.Itoa(number - 1)
}

func testServerHostPort(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("TOOLS_TESTING_MONGOD")
	if uri == "" {
		return "localhost:" + db.DefaultTestPort
	}
	parsed, err := url.Parse(uri)
	require.NoError(t, err, "TOOLS_TESTING_MONGOD is a URI")
	require.NotEmpty(t, parsed.Host, "TOOLS_TESTING_MONGOD names a host")
	return parsed.Host
}
