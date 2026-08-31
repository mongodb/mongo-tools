// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongostat

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/internal/testcmd"
	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mongostat has no in-process entry point: main() does its own option
// validation and calls os.Exit, so these tests build the tool once and run it as
// a subprocess, reading its exit status and output.

// defaultHeaderPrefix is the first column of the default output, which is what
// tells a header line from a data row: data rows start with a count.
const defaultHeaderPrefix = "insert"

// TestMongostatRowCount checks that --rowcount prints exactly that many data
// rows and exits successfully, and covers the ways of naming a server that
// mongostat has to reject.
func TestMongostatRowCount(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	t.Run("prints the requested number of rows", func(t *testing.T) {
		const rowCount = 5
		stdout, stderr, err := runMongostat(t, "--rowcount", strconv.Itoa(rowCount), "--noheaders")
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)
		assert.Len(t, testcmd.Rows(stdout), rowCount, "--rowcount is the number of rows printed")
	})

	t.Run("a rowcount that is not a number is rejected", func(t *testing.T) {
		_, stderr, err := runMongostat(t, "--rowcount", "foobar")
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(t, stderr, "try 'mongostat --help'", "the failure names the bad option")
	})

	t.Run("a server that cannot be reached is an error", func(t *testing.T) {
		// Without a shortened selection timeout this case spends the driver's
		// default 30 seconds waiting for a server that is never coming.
		_, stderr, err := testcmd.Run(t, mongostatBinary(t), slices.Concat(
			testopts.GetSSLArgs(), testopts.GetAuthArgs(),
			[]string{
				"--host", "localhost", "--port", testcmd.UnreachablePort(t),
				"--serverSelectionTimeout", "2",
				"--rowcount", "1",
			},
		)...)
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(t, stderr, "failed to connect",
			"the failure is the connection, not something else")
	})

	t.Run("a replica set name that does not match is an error", func(t *testing.T) {
		_, stderr, err := testcmd.Run(t, mongostatBinary(t), slices.Concat(
			testopts.GetSSLArgs(), testopts.GetAuthArgs(),
			[]string{
				"--host", "badreplset/" + testcmd.ServerHostPort(t),
				"--serverSelectionTimeout", "2",
				"--rowcount", "1",
			},
		)...)
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(t, stderr, "failed to connect",
			"the failure is the connection, not something else")
	})
}

// TestMongostatSleepTime checks that the positional sleep-time argument spaces
// the rows out. mongostat needs two samples of the server's counters before it
// can print a row, so n rows take (n+1) sleeps: the assertion allows for one
// fewer than that, which still fails if the argument were ignored and the
// default one-second interval used instead.
func TestMongostatSleepTime(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		rowCount     = 2
		sleepSeconds = 2
	)
	start := time.Now()
	stdout, stderr, err := runMongostat(t,
		"--rowcount", strconv.Itoa(rowCount), "--noheaders", strconv.Itoa(sleepSeconds),
	)
	elapsed := time.Since(start)
	require.NoError(t, err, "mongostat exits successfully: %s", stderr)

	assert.Len(t, testcmd.Rows(stdout), rowCount, "every row is printed")
	assert.GreaterOrEqual(t, elapsed, rowCount*sleepSeconds*time.Second,
		"each row waits out the sleep time")
}

// TestMongostatHeaders checks that the column header is printed by default and
// suppressed by --noheaders.
func TestMongostatHeaders(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	for _, c := range []struct {
		name     string
		args     []string
		wantRows int
	}{
		{"a header is printed by default", nil, 2},
		{"noheaders suppresses it", []string{"--noheaders"}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runMongostat(t, append(c.args, "--rowcount", "1")...)
			require.NoError(t, err, "mongostat exits successfully: %s", stderr)

			rows := testcmd.Rows(stdout)
			require.Len(t, rows, c.wantRows, "the expected number of lines is printed")
			hasHeader := strings.HasPrefix(rows[0], defaultHeaderPrefix)
			assert.Equal(t, c.wantRows == 2, hasHeader,
				"the header is present only by default, got %#q", rows[0])
		})
	}
}

// TestMongostatCustomColumns checks the -o and -O column selectors: which
// columns appear, that a column can be renamed with name=alias, and that a
// serverStatus field that is not one of the built-in columns is read from the
// server.
func TestMongostatCustomColumns(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	// The default time format contains spaces, so a row whose last column is the
	// time splits into more fields than there are columns. --humanReadable=false
	// prints an RFC 3339 timestamp instead, which is a single field.
	for _, c := range []struct {
		name       string
		args       []string
		wantHeader []string
		wantFields int
	}{
		{
			"selects the columns",
			[]string{"-o", "host,conn,time", "--humanReadable=false"},
			[]string{"host", "conn", "time"}, 3,
		},
		{
			"the default time format takes three fields",
			[]string{"-o", "host,conn,time"},
			[]string{"host", "conn", "time"}, 5,
		},
		{
			"renames the columns",
			[]string{"-o", "host=H,conn=C,time=MYTiME", "--humanReadable=false"},
			[]string{"H", "C", "MYTiME"}, 3,
		},
		{
			"renames a serverStatus column",
			[]string{"-o", "host,conn=MYCoNN,mem.bits=BiTs"},
			[]string{"host", "MYCoNN", "BiTs"}, 3,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runMongostat(t, append(c.args, "-n", "1")...)
			require.NoError(t, err, "mongostat exits successfully: %s", stderr)

			rows := testcmd.Rows(stdout)
			require.Len(t, rows, 2, "the output is a header and one data row")
			assert.Equal(t, c.wantHeader, strings.Fields(rows[0]),
				"the header names the selected columns")
			assert.Len(t, strings.Fields(rows[1]), c.wantFields,
				"the data row has the expected number of fields")
		})
	}

	t.Run("o and O together are rejected", func(t *testing.T) {
		_, stderr, err := runMongostat(t, "-o", "host", "-O", "conn", "-n", "1")
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(t, stderr, "-O cannot be used if -o is also specified",
			"the failure names the conflict")
	})

	t.Run("o reads the value of a serverStatus field", func(t *testing.T) {
		stdout, stderr, err := runMongostat(t, "-o", "host,conn,mem.bits", "-n", "1")
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)

		rows := testcmd.Rows(stdout)
		require.Len(t, rows, 2, "the output is a header and one data row")
		fields := strings.Fields(rows[1])
		require.Len(t, fields, 3, "the data row has one field per column")
		assert.Contains(t, []string{"32", "64"}, fields[2],
			"mem.bits comes from the server, so it is a word size")
	})

	t.Run("O appends to the default columns", func(t *testing.T) {
		stdout, stderr, err := runMongostat(t, "-O", "host", "-n", "1")
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)

		rows := testcmd.Rows(stdout)
		require.Len(t, rows, 2, "the output is a header and one data row")
		assert.True(t, strings.HasPrefix(rows[0], defaultHeaderPrefix),
			"the default columns are still there, got %#q", rows[0])
		header := strings.Fields(rows[0])
		assert.Equal(t, "host", header[len(header)-1], "the added column comes last")
	})
}

// TestMongostatExitsOnSignal checks that a mongostat left running until it is
// signaled reports a failure exit status. main passes a nil finalizer to
// signals.Handle, so the first signal reaches the os.Exit(ExitFailure) in
// handleSignals.
func TestMongostatExitsOnSignal(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	if runtime.GOOS == "windows" {
		t.Skip("Skipping test because Windows does not support sending SIGTERM to a process")
	}

	// No --rowcount, so it runs until signaled.
	cmd := exec.Command(mongostatBinary(t), append(testopts.GetBareArgs(), "--noheaders")...)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "can read mongostat's output")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start(), "mongostat starts")

	firstRow := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			firstRow <- scanner.Text()
		}
		close(firstRow)
	}()
	select {
	case row, ok := <-firstRow:
		if !ok {
			t.Fatalf(
				"mongostat printed a row before being signaled: %s",
				stderrAfterExit(cmd, &stderr),
			)
		}
		require.NotEmpty(t, strings.Fields(row), "the row has content")
	case <-time.After(30 * time.Second):
		t.Fatalf("mongostat printed no rows: %s", stderrAfterExit(cmd, &stderr))
	}

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM), "can signal mongostat")

	err = cmd.Wait()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "mongostat exits nonzero when signaled")
	assert.Equal(t, util.ExitFailure, exitErr.ExitCode(),
		"a signal is reported as a failure exit status")
}

// os/exec fills a Stderr buffer from a goroutine it starts in Start, so nothing
// may read that buffer until Wait returns. Both failure paths above still have
// a running (or just-exited) mongostat, so this stops it first. Kill and Wait
// errors are ignored because the caller is already failing the test and the
// stderr text is more useful than either of them.
func stderrAfterExit(cmd *exec.Cmd, stderr *bytes.Buffer) string {
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	return stderr.String()
}

// TestMongostatAuth checks that mongostat authenticates with the credentials it
// is given, and fails when the password is wrong rather than reporting stats
// anyway.
func TestMongostatAuth(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	// The credentials are supplied here rather than by GetBareArgs, which would
	// supply the right password for both cases.
	auth := testopts.GetAuthOptions()
	args := slices.Concat(testopts.GetSSLArgs(), []string{
		"--host", testcmd.ServerHostPort(t),
		"--authenticationDatabase", auth.Source,
		"--username", auth.Username,
		"--serverSelectionTimeout", "5",
		"--rowcount", "1",
	})

	t.Run("the right password succeeds", func(t *testing.T) {
		stdout, stderr, err := testcmd.Run(t, mongostatBinary(t),
			slices.Concat(args, []string{"--password", auth.Password})...)
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)
		assert.Len(t, testcmd.Rows(stdout), 2, "a header and one data row are printed")
	})

	t.Run("a wrong password fails", func(t *testing.T) {
		stdout, stderr, err := testcmd.Run(t, mongostatBinary(t),
			slices.Concat(args, []string{"--password", "not-the-password"})...)
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(t, stderr, "Authentication failed",
			"the failure is the authentication, not something else")
		assert.Empty(t, testcmd.Rows(stdout), "no stats are printed")
	})
}

// TestMongostatDiscoverShards checks that --discover against a mongos reports on
// every shard, not just the mongos it was pointed at.
func TestMongostatDiscoverShards(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)

	shardHosts := shardHostsFromConfig(t)
	require.NotEmpty(t, shardHosts, "the cluster has shards to discover")

	// Discovery happens on the first poll of the mongos, but a host needs two
	// samples before it produces a row, so the first rows cover only the seed.
	// Enough rows are requested for every shard to have been picked up.
	stdout, stderr, err := runMongostat(t, "--discover", "--rowcount", "8", "--noheaders")
	require.NoError(t, err, "mongostat exits successfully: %s", stderr)

	// A host whose poll failed still gets a row, holding the error text instead
	// of counters, so a row only counts as reporting if it carries a count.
	reported := make(map[string]bool)
	for _, row := range testcmd.Rows(stdout) {
		fields := strings.Fields(row)
		if len(fields) > 1 && countedField(fields[1]) {
			reported[fields[0]] = true
		}
	}
	for _, host := range shardHosts {
		assert.True(t, reported[host],
			"--discover reports counters for shard %#q, saw %v", host, reported)
	}
}

// countedField reports whether a field is one of mongostat's counts, which are
// either a number or a number prefixed with * for an opcounter repl value.
func countedField(field string) bool {
	_, err := strconv.Atoi(strings.TrimPrefix(field, "*"))
	return err == nil
}

// shardHostsFromConfig returns the host:port of every shard member, read from
// config.shards, where each host is recorded as "setName/host:port,host:port".
func shardHostsFromConfig(t *testing.T) []string {
	t.Helper()
	client, err := testutil.GetBareSession(t)
	require.NoError(t, err, "can connect to the cluster")
	defer func() {
		require.NoError(t, client.Disconnect(t.Context()), "can disconnect")
	}()

	cursor, err := client.Database("config").Collection("shards").Find(t.Context(), bson.D{})
	require.NoError(t, err, "can read config.shards")
	defer cursor.Close(t.Context())

	var hosts []string
	for cursor.Next(t.Context()) {
		var shard struct {
			Host string `bson:"host"`
		}
		require.NoError(t, cursor.Decode(&shard))
		members := shard.Host
		if _, after, found := strings.Cut(members, "/"); found {
			members = after
		}
		hosts = append(hosts, strings.Split(members, ",")...)
	}
	require.NoError(t, cursor.Err(), "can read every shard")
	return hosts
}

// runMongostat runs mongostat against the test server. Stat rows go to stdout
// and everything else to stderr, so they are returned separately rather than
// merged: a diagnostic line would otherwise be read as a stat row.
func runMongostat(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return testcmd.RunAgainstTestServer(t, mongostatBinary(t), args...)
}

// buildMongostat builds the tool once for the whole package. Building rather
// than `go run` keeps compile time out of the tests that measure elapsed time,
// and means a build failure cannot be mistaken for the tool exiting nonzero. The
// binary needs the platform's extension, because Windows cannot exec a path
// without one.
var buildMongostat = sync.OnceValues(func() (string, error) {
	binary := filepath.Join(os.TempDir(), "mongostat-integration-test"+platform.GetLocalBinaryExt())
	if out, err := exec.Command("go", "build", "-o", binary, "./main").
		CombinedOutput(); err != nil {
		return "", fmt.Errorf("building mongostat: %v: %s", err, out)
	}
	return binary, nil
})

func mongostatBinary(t *testing.T) string {
	t.Helper()
	binary, err := buildMongostat()
	require.NoError(t, err, "mongostat builds")
	return binary
}
