// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build sharded

package mongodump

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOplogDumpFailsOnSharded checks that `--oplog` against a sharded cluster fails. Mongodump does
// not support oplog dump with sharded clusters.
func TestOplogDumpFailsOnSharded(t *testing.T) {
	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	md.ToolOptions.DB = ""
	md.OutputOptions.Oplog = true
	md.OutputOptions.Out = t.TempDir()
	err = md.Init()
	require.ErrorContains(
		t, err, "can't use --oplog option when dumping from a mongos",
		"an --oplog dump of a sharded cluster fails",
	)

	entries, err := os.ReadDir(md.OutputOptions.Out)
	require.NoError(t, err, "can read the dump directory")
	assert.Empty(t, entries, "the failed dump wrote nothing")
}
