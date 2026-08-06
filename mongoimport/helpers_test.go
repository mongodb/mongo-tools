// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// openTestFixture is shared by csv_test.go, json_test.go, and tsv_test.go.
// It registers its own teardown so each caller gets a fresh handle.
func openTestFixture(t *testing.T, path string) *os.File {
	t.Helper()

	fileHandle, err := os.Open(path)
	require.NoError(t, err, "should open the test fixture")
	t.Cleanup(func() { fileHandle.Close() })

	return fileHandle
}
