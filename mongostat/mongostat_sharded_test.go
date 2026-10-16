// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build sharded

package mongostat

import (
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/internal/testcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMongostatDiscoverShards checks that `--discover` against a mongos reports on every shard, not
// just the mongos it was pointed at.
func TestMongostatDiscoverShards(t *testing.T) {
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
