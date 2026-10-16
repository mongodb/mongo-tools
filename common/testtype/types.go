// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package testtype holds the runtime configuration checks that integration tests use to select a
// cluster configuration. Which tests compile and run is chosen with build tags (see the
// buildscript's test:* targets); these env vars only describe the server the tests are pointed at.
package testtype

import (
	"os"
	"testing"
)

const (
	// SSLTestType is set when the tests run against a TLS-enabled server.
	SSLTestType = "TOOLS_TESTING_SSL"

	// AuthTestType is set when the tests run against an auth-enabled server.
	AuthTestType = "TOOLS_TESTING_AUTH"
)

func HasTestType(testType string) bool {
	envVal := os.Getenv(testType)
	return envVal != ""
}

// Skip the test if the specified configuration is not in effect.
func SkipUnlessTestType(t *testing.T, testType string) {
	if !HasTestType(testType) {
		t.Logf("The environment lacks %q; skipping this test …", testType)
		t.SkipNow()
	}
}
