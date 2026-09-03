// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package dsctest provides test helpers for running against a server that uses disaggregated
// storage (DSC). It lives in its own package, importing only the detector and the driver, so that
// both common/testutil and tests inside common/db can use it without an import cycle.
package dsctest

import (
	"context"
	"testing"

	"github.com/mongodb/mongo-tools/common/db/dsc"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SkipForDisaggregatedStorage will skip the test if the server is running with disaggregated
// storage (DSC) enabled.
//
// Use this for tests that depend on a server feature DSC does not support, naming that feature in
// the reason. It deliberately keys off DSC rather than off the unsupported operation failing, so
// an unexpected failure of that same operation elsewhere still fails the test loudly instead of
// quietly skipping it.
func SkipForDisaggregatedStorage(t *testing.T, client *mongo.Client, reason string) {
	t.Helper()

	isDSC, err := dsc.IsDisaggregatedStorage(t.Context(), client)
	require.NoError(t, err, "checking for disaggregated storage")

	if isDSC {
		t.Skipf("Skipping test because the server uses disaggregated storage: %s", reason)
	}
}

// SkipUnlessDisaggregatedStorage will skip the test if the server is not running with
// disaggregated storage (DSC) enabled.
//
// Use this for tests that exercise a guardrail that only fires on a DSC cluster, so they only run
// against the DSC CI variant.
func SkipUnlessDisaggregatedStorage(t *testing.T, client *mongo.Client, reason string) {
	t.Helper()

	isDSC, err := dsc.IsDisaggregatedStorage(context.Background(), client)
	require.NoError(t, err, "checking for disaggregated storage")

	if !isDSC {
		t.Skipf("Skipping test because the server does not use disaggregated storage: %s", reason)
	}
}
