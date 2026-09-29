// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongostat

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/internal/testcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMongostatJSON checks that --json prints one JSON object per sample, keyed
// by the host it reports on, and that the object carries the default columns as
// fields with values read from the server.
func TestMongostatJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const rowCount = 2
	stdout, stderr, err := runMongostat(t, "--json", "--rowcount", strconv.Itoa(rowCount))
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

// TestMongostatTableValues checks that a table data row carries values read
// from the server rather than blanks. It selects its columns rather than using
// the defaults, because a default column with no value on this server would
// leave an empty cell and pull the rest of the row out of alignment.
// --humanReadable=false keeps the timestamp free of the spaces the default
// format uses, so the row has exactly one field per column.
func TestMongostatTableValues(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const columns = "host,conn,time"
	stdout, stderr, err := runMongostat(
		t,
		"-o", columns, "--rowcount", "1", "--humanReadable=false",
	)
	require.NoError(t, err, "mongostat exits successfully: %s", stderr)

	rows := testcmd.Rows(stdout)
	require.Len(t, rows, 2, "the output is a header and one data row")
	assert.Equal(
		t,
		strings.Split(columns, ","),
		strings.Fields(rows[0]),
		"the header names the selected columns",
	)

	values := strings.Fields(rows[1])
	require.Len(t, values, 3, "the data row has one field per column, got %#q", rows[1])
	host, conn, sampledAt := values[0], values[1], values[2]

	assert.NotEmpty(t, host, "the data row names the host it reports on")
	assert.Contains(t, host, ":", "the reported host includes its port")

	_, err = strconv.Atoi(conn)
	assert.NoError(t, err, "the connection count is a number, got %#q", conn)

	_, err = time.Parse(time.RFC3339, sampledAt)
	assert.NoError(t, err, "the sample time is an RFC 3339 timestamp, got %#q", sampledAt)
}
