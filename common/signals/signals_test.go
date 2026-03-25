// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package signals

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/mongodb/mongo-tools/common/testutil"
)

// TestMain handles the subprocess mode used by TestBrokenPipeHandledAsWriteError.
func TestMain(m *testing.M) {
	if os.Getenv("TEST_BROKEN_PIPE_SUBPROCESS") == "1" {
		// Set up SIGPIPE handling as our tools do.
		done := HandleWithInterrupt(nil)
		defer close(done)

		// Write repeatedly to stdout. When the reader closes the pipe,
		// the write will return an EPIPE error. We exit 0 to signal that
		// the error was surfaced and handled — not a SIGPIPE death.
		for {
			_, err := fmt.Println("data")
			if err != nil {
				os.Exit(0)
			}
		}
	}
	os.Exit(m.Run())
}

// TestBrokenPipeHandledAsWriteError verifies that when the read end of a pipe
// is closed, our signal handler causes the write error to surface as an EPIPE
// (allowing the tool to exit cleanly) rather than the process being killed by
// SIGPIPE.
func TestBrokenPipeHandledAsWriteError(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestBrokenPipeHandledAsWriteError")
	cmd.Env = append(os.Environ(), "TEST_BROKEN_PIPE_SUBPROCESS=1")
	testutil.AssertBrokenPipeHandled(t, cmd)
}
