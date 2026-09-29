// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongostat

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// mongostat has no in-process entry point: main() does its own option validation and calls os.Exit,
// so these tests build the tool once and run it as a subprocess, reading its exit status and
// output.

// TestMongostatRowCount checks that `--rowcount` prints exactly that many data rows and exits
// successfully, and covers the ways of naming a server that mongostat has to reject.
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
		// Without a shortened selection timeout this case spends the driver's default 30 seconds
		// waiting for a server that is never coming.
		_, stderr, err := testcmd.Run(t, mongostatBinary(t), slices.Concat(
			testopts.GetSSLArgs(), testopts.GetAuthArgs(),
			[]string{
				"--host", "localhost",
				"--port", testcmd.UnreachablePort(t),
				"--serverSelectionTimeout", "2",
				"--rowcount", "1",
			},
		)...)
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(
			t,
			stderr,
			"failed to connect",
			"the failure is the connection, not something else",
		)
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

// TestMongostatSleepTime checks that the positional sleep-time argument spaces the rows
// out. mongostat needs two samples of the server's counters before it can print a row, so n rows
// take (n+1) sleeps: the assertion allows for one fewer than that, which still fails if the
// argument were ignored and the default one-second interval used instead.
func TestMongostatSleepTime(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		rowCount     = 2
		sleepSeconds = 2
	)
	start := time.Now()
	stdout, stderr, err := runMongostat(
		t,
		"--rowcount", strconv.Itoa(rowCount),
		"--noheaders", strconv.Itoa(sleepSeconds),
	)
	elapsed := time.Since(start)
	require.NoError(t, err, "mongostat exits successfully: %s", stderr)

	assert.Len(t, testcmd.Rows(stdout), rowCount, "every row is printed")
	assert.GreaterOrEqual(
		t,
		elapsed,
		rowCount*sleepSeconds*time.Second,
		"each row waits out the sleep time",
	)
}

// TestMongostatHeaders checks that the column header is printed by default and suppressed by
// `--noheaders`.
func TestMongostatHeaders(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	for _, c := range []struct {
		name         string
		args         []string
		expectHeader bool
	}{
		{"a header is printed by default", nil, true},
		{"noheaders suppresses it", []string{"--noheaders"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runMongostat(t, append(c.args, "--rowcount", "1")...)
			require.NoError(t, err, "mongostat exits successfully: %s", stderr)

			rows := testcmd.Rows(stdout)
			if c.expectHeader {
				assert.Len(t, rows, 2, "got a header by default")
				testHeaderColumnsArePresent(t, rows[0], defaultHeaderColumns()...)
			} else {
				assert.Len(t, rows, 1, "no header when suppressed")
			}
		})
	}
}

// TestMongostatCustomColumns checks the `-o` and `-O` column selectors: which columns appear, that
// a column can be renamed with `name=alias`, and that a `serverStatus` field that is not one of the
// built-in columns is read from the server.
func TestMongostatCustomColumns(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	// The default time format contains spaces, so a row whose last column is the time splits into
	// more fields than there are columns. `--humanReadable=false` prints an RFC 3339 timestamp
	// instead, which is a single field.
	for _, c := range []struct {
		name       string
		args       []string
		wantHeader []string
		wantFields int
		// wantLastFieldIn, when set, lists the values the row's last field may
		// take. It checks that a column renamed with name=alias still reads the
		// serverStatus field it names.
		wantLastFieldIn []string
	}{
		{
			"selects the columns",
			[]string{"-o", "host,conn,time", "--humanReadable=false"},
			[]string{"host", "conn", "time"},
			3,
			nil,
		},
		{
			"the default time format takes three fields",
			[]string{"-o", "host,conn,time"},
			[]string{"host", "conn", "time"},
			5,
			nil,
		},
		{
			"renames the columns",
			[]string{"-o", "host=H,conn=C,time=MYTiME", "--humanReadable=false"},
			[]string{"H", "C", "MYTiME"},
			3,
			nil,
		},
		{
			"renames a serverStatus column",
			[]string{"-o", "host,conn=MYCoNN,mem.bits=BiTs"},
			[]string{"host", "MYCoNN", "BiTs"},
			3,
			[]string{"32", "64"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runMongostat(t, append(c.args, "-n", "1")...)
			require.NoError(t, err, "mongostat exits successfully: %s", stderr)

			rows := testcmd.Rows(stdout)
			require.Len(t, rows, 2, "the output is a header and one data row")
			assert.Equal(
				t,
				c.wantHeader,
				strings.Fields(rows[0]),
				"the header names the selected columns",
			)
			fields := strings.Fields(rows[1])
			assert.Len(
				t,
				fields,
				c.wantFields,
				"the data row has the expected number of fields",
			)
			if len(c.wantLastFieldIn) > 0 {
				assert.Contains(
					t,
					c.wantLastFieldIn,
					fields[len(fields)-1],
					"the renamed serverStatus column reads its value from the server",
				)
			}
		})
	}

	t.Run("the selected columns carry values read from the server", func(t *testing.T) {
		stdout, stderr, err := runMongostat(
			t,
			"-o",
			"host,conn,time",
			"--humanReadable=false",
			"-n",
			"1",
		)
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)

		rows := testcmd.Rows(stdout)
		require.Len(t, rows, 2, "the output is a header and one data row")
		fields := strings.Fields(rows[1])
		require.Len(t, fields, 3, "the data row has one field per column")
		host, conn, sampledAt := fields[0], fields[1], fields[2]

		assert.Contains(t, host, ":", "the host column names the host and its port")
		_, err = strconv.Atoi(conn)
		assert.NoError(t, err, "the connection count is a number, got %#q", conn)
		_, err = time.Parse(time.RFC3339, sampledAt)
		assert.NoError(t, err, "the sample time is an RFC 3339 timestamp, got %#q", sampledAt)
	})

	t.Run("o and O together are rejected", func(t *testing.T) {
		_, stderr, err := runMongostat(t, "-o", "host", "-O", "conn", "-n", "1")
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(
			t,
			stderr,
			"-O cannot be used if -o is also specified",
			"the failure names the conflict",
		)
	})

	t.Run("o reads the value of a serverStatus field", func(t *testing.T) {
		stdout, stderr, err := runMongostat(t, "-o", "host,conn,mem.bits", "-n", "1")
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)

		rows := testcmd.Rows(stdout)
		require.Len(t, rows, 2, "the output is a header and one data row")
		fields := strings.Fields(rows[1])
		require.Len(t, fields, 3, "the data row has one field per column")
		assert.Contains(
			t,
			[]string{"32", "64"},
			fields[2],
			"mem.bits comes from the server, so it is a word size",
		)
	})

	t.Run("O appends to the default columns", func(t *testing.T) {
		stdout, stderr, err := runMongostat(t, "-O", "host", "-n", "1")
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)

		rows := testcmd.Rows(stdout)
		require.Len(t, rows, 2, "the output is a header and one data row")
		testHeaderColumnsArePresent(t, rows[0], append(defaultHeaderColumns(), "host")...)
		header := strings.Fields(rows[0])
		assert.Equal(t, "host", header[len(header)-1], "the added column comes last")
	})
}

// TestMongostatJSON checks that `--json` prints one JSON object per sample, keyed by the host it
// reports on, and that the object carries the default columns as fields with values read from the
// server. It connects to a single host: with a multi-host deployment mongostat monitors
// asynchronously and can re-print a sample it already printed as `{"error":"no data received"}`,
// which would make the per-sample value checks flaky.
func TestMongostatJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const rowCount = 2
	stdout, stderr, err := runMongostatAgainstServer(
		t,
		testcmd.SingleHostURI(t),
		"--json",
		"--rowcount",
		strconv.Itoa(rowCount),
	)
	require.NoError(t, err, "mongostat exits successfully: %s", stderr)

	rows := testcmd.Rows(stdout)
	require.Len(t, rows, rowCount, "--rowcount is the number of samples printed")

	for i, row := range rows {
		var sample map[string]map[string]string
		require.NoError(t, json.Unmarshal([]byte(row), &sample), "sample %d is JSON: %s", i, row)
		require.NotEmpty(t, sample, "sample %d reports on at least one host", i)

		for host, fields := range sample {
			assert.NotEmpty(t, host, "sample %d keys its report by host", i)
			require.NotContains(t, fields, "error", "sample %d reports stats for %s", i, host)

			sampledAt, ok := fields["time"]
			require.True(t, ok, "the sample for %s has a time", host)
			assert.NotEmpty(t, sampledAt, "the sample time for %s is set", host)

			conn, ok := fields["conn"]
			require.True(t, ok, "the sample for %s has a connection count", host)
			_, err := strconv.Atoi(conn)
			assert.NoError(t, err, "the connection count for %s is a number, got %#q", host, conn)
		}
	}
}

// defaultHeaderColumns returns the columns mongostat prints in its default header. A replica set
// adds the set and repl columns that a standalone does not have. The host column is not part of the
// default header: it only appears with --discover or when monitoring more than one host.
func defaultHeaderColumns() []string {
	cols := []string{"insert", "conn", "time"}
	if testtype.HasTestType(testtype.ReplSetTestType) {
		cols = append(cols, "set", "repl")
	}
	return cols
}

func testHeaderColumnsArePresent(t *testing.T, header string, cols ...string) {
	for _, col := range cols {
		re := regexp.MustCompile(fmt.Sprintf(`\b%s\b`, col))
		assert.Regexp(
			t,
			re,
			header,
			"header row contains a column named %#q",
			col,
		)
	}
}

// TestMongostatExitsOnSignal checks that a mongostat left running until it is signaled reports a
// failure exit status. main passes a nil finalizer to signals.Handle, so the first signal reaches
// the os.Exit(ExitFailure) in handleSignals.
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
	assert.Equal(
		t,
		util.ExitFailure,
		exitErr.ExitCode(),
		"a signal is reported as a failure exit status",
	)
}

// `os/exec` fills a `Stderr` buffer from a goroutine it starts in `Start`, so nothing can read that
// buffer until `Wait` returns. Both failure paths above still have a running (or just-exited)
// mongostat, so this stops it first. `Kill` and `Wait` errors are ignored because the caller is
// already failing the test and the stderr text is more useful than either of them.
func stderrAfterExit(cmd *exec.Cmd, stderr *bytes.Buffer) string {
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	return stderr.String()
}

// TestMongostatAuth checks that mongostat authenticates with the credentials it is given, and fails
// when the password is wrong rather than reporting stats anyway.
func TestMongostatAuth(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.AuthTestType)

	// The credentials are supplied here rather than by GetBareArgs, which would supply the right
	// password for both cases.
	auth := testopts.GetAuthOptions()
	args := slices.Concat(testopts.GetSSLArgs(), []string{
		"--host", testcmd.ServerHostPort(t),
		"--authenticationDatabase", auth.Source,
		"--username", auth.Username,
		"--serverSelectionTimeout", "5",
		"--rowcount", "1",
	})

	t.Run("the right password succeeds", func(t *testing.T) {
		stdout, stderr, err := testcmd.Run(
			t,
			mongostatBinary(t),
			slices.Concat(args, []string{"--password", auth.Password})...)
		require.NoError(t, err, "mongostat exits successfully: %s", stderr)
		assert.Len(t, testcmd.Rows(stdout), 2, "a header and one data row are printed")
	})

	t.Run("a wrong password fails", func(t *testing.T) {
		stdout, stderr, err := testcmd.Run(
			t,
			mongostatBinary(t),
			slices.Concat(args, []string{"--password", "not-the-password"})...)
		testcmd.RequireExitFailure(t, err, "mongostat")
		assert.Contains(
			t,
			stderr,
			"Authentication failed",
			"the failure is the authentication, not something else",
		)
		assert.Empty(t, testcmd.Rows(stdout), "no stats are printed")
	})
}

// TestMongostatDiscoverShards checks that `--discover` against a mongos reports on every shard, not
// just the mongos it was pointed at.
func TestMongostatDiscoverShards(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)

	shardHosts := shardHostsFromConfig(t)
	require.NotEmpty(t, shardHosts, "the cluster has shards to discover")

	// Discovery happens on the first poll of the mongos, but a host needs two samples before it
	// produces a row, so the first rows cover only the seed.  Enough rows are requested for every
	// shard to have been picked up.
	stdout, stderr, err := runMongostat(t, "--discover", "--rowcount", "8", "--noheaders")
	require.NoError(t, err, "mongostat exits successfully: %s", stderr)

	// A host whose poll failed still gets a row, holding the error text instead of counters, so a
	// row only counts as reporting if it carries a count.
	reported := make(map[string]bool)
	for _, row := range testcmd.Rows(stdout) {
		fields := strings.Fields(row)
		if len(fields) > 1 && countedField(fields[1]) {
			reported[fields[0]] = true
		}
	}
	for _, host := range shardHosts {
		assert.True(
			t,
			reported[host],
			"--discover reports counters for shard %#q, saw %v",
			host,
			reported,
		)
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

// runMongostat runs mongostat against the test server. Stat rows go to stdout and everything else
// to stderr, so they are returned separately rather than merged: a diagnostic line would otherwise
// be read as a stat row.
func runMongostat(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return testcmd.RunAgainstTestServer(t, mongostatBinary(t), args...)
}

// runMongostatAgainstServer runs mongostat against one server URI rather than the whole test
// deployment. runMongostat points at every host the deployment advertises, which on a replica set
// is more than one.
func runMongostatAgainstServer(
	t *testing.T,
	uri string,
	args ...string,
) (string, string, error) {
	t.Helper()
	return testcmd.Run(t, mongostatBinary(t), append(testopts.GetBareArgsForURI(uri), args...)...)
}

// buildMongostat builds the tool once for the whole package. Building rather than `go run` keeps
// compile time out of the tests that measure elapsed time, and means a build failure cannot be
// mistaken for the tool exiting nonzero. The binary needs the platform's extension, because Windows
// cannot exec a path without one.
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
