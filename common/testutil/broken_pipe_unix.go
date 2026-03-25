// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build !windows

package testutil

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// AssertBrokenPipeHandled starts cmd with its stdout connected to a pipe,
// reads a small amount of output to confirm the process has started writing,
// then breaks the pipe by closing the read end. It asserts the process was
// not killed by SIGPIPE — a broken pipe must surface as a write error that
// the process can handle cleanly.
//
// cmd.Stdout must not be set before calling; this function sets it to the
// write end of a pipe.
func AssertBrokenPipeHandled(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	cmd.Stdout = pw

	require.NoError(t, cmd.Start())
	require.NoError(t, pw.Close())

	buf := make([]byte, 64)
	_, err = pr.Read(buf)
	require.NoError(t, err, "process should write at least some output before the pipe breaks")
	require.NoError(t, pr.Close())

	err = cmd.Wait()
	if err == nil {
		return
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		require.True(t, ok, "exitErr.Sys() should return a syscall.WaitStatus")

		if status.Signaled() {
			require.NotEqual(
				t,
				syscall.SIGPIPE,
				status.Signal(),
				"process should not be killed by SIGPIPE: broken pipe should surface as a write error",
			)
		}
	}
}
