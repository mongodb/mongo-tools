// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build sharded

package mongorestore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApplyOpsGuardrail checks that `--oplogReplay` and `--preserveUUID` fail fast against a
// sharded cluster, before any data is restored, since mongorestore implements both on top of the
// `applyOps` command that shardec clusters do not support.
func TestApplyOpsGuardrail(t *testing.T) {
	t.Run("--oplogReplay is rejected", func(t *testing.T) {
		result := runRestoreFromArgs(t, OplogReplayOption, t.TempDir())
		require.ErrorContains(
			t, result.Err, "cannot use --oplogReplay",
			"--oplogReplay against a sharded or DSC cluster errors",
		)
	})

	t.Run("--preserveUUID is rejected", func(t *testing.T) {
		result := runRestoreFromArgs(t, PreserveUUIDOption, DropOption, t.TempDir())
		require.ErrorContains(
			t, result.Err, "cannot use --preserveUUID",
			"--preserveUUID against a sharded or DSC cluster errors",
		)
	})
}
