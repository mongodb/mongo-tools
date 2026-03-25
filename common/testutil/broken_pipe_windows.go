// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build windows

package testutil

import (
	"os/exec"
	"testing"
)

// AssertBrokenPipeHandled is a no-op on Windows where SIGPIPE does not apply.
func AssertBrokenPipeHandled(t *testing.T, cmd *exec.Cmd) {
	t.Skip("broken pipe handling via SIGPIPE is not applicable on Windows")
}
