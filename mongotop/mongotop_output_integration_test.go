// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongotop

import (
	"context"
	"regexp"
	"strconv"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/internal/testcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMongotopGrid checks the default table output, which the JSON coverage
// does not: a header naming the ns, total, read, and write columns, and a row
// for a namespace carrying a nonzero time.
func TestMongotopGrid(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongotop_grid_test"
		collName = "busy"
		rowCount = 4
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

	stopWorkload := workUntilStopped(t, client.Database(dbName).Collection(collName))

	stdout, stderr, err := runMongotop(t, "--rowcount", strconv.Itoa(rowCount))
	stopWorkload()
	require.NoError(t, err, "mongotop exits successfully: %s", stderr)

	header := regexp.MustCompile(`^ns\s+total\s+read\s+write\b`)
	activityRow := regexp.MustCompile(`^\S+\s+(-?\d+)ms\s+(-?\d+)ms\s+(-?\d+)ms$`)

	var headerSeen, activitySeen bool
	for _, line := range testcmd.Rows(stdout) {
		if header.MatchString(line) {
			headerSeen = true
			continue
		}
		times := activityRow.FindStringSubmatch(line)
		if times == nil {
			continue
		}
		for _, sampled := range times[1:] {
			if sampled != "0" {
				activitySeen = true
			}
		}
	}

	assert.True(
		t,
		headerSeen,
		"the grid has a header naming ns, total, read, and write:\n%s",
		stdout,
	)
	assert.True(
		t,
		activitySeen,
		"the grid reports a nonzero time for a namespace:\n%s",
		stdout,
	)
}
