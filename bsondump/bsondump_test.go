// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsondump

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/require"
)

func TestBsondump(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("bsondump reading from stdin and writing to stdout", testFromStdinToStdout)
	t.Run("bsondump reading from stdin and writing to a file", testFromStdinToFile)
	t.Run(
		"bsondump reading from a file with --bsonFile and writing to stdout",
		testFromFileWithNamedArgumentToStdout,
	)
	t.Run(
		"bsondump reading from a file with a positional arg and writing to stdout",
		testFromFileWithPositionalArgumentToStdout,
	)
	t.Run(
		"bsondump reading from a file with --bsonFile and writing to a file",
		testFromFileWithNamedArgumentToFile,
	)
	t.Run(
		"bsondump reading from a file with a positional arg and writing to a file",
		testFromFileWithPositionalArgumentToFile,
	)
}

func testFromStdinToStdout(t *testing.T) {
	require := require.New(t)
	cmd := bsondumpCommand()

	// Attach a file to stdin of the command.
	inFile, err := os.Open("testdata/sample.bson")
	require.NoError(err)
	cmd.Stdin = inFile

	// Attach a buffer to stdout of the command.
	cmdOutput := &bytes.Buffer{}
	cmd.Stdout = cmdOutput

	err = cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	require.NoError(err)
	bufRef := new(bytes.Buffer)
	bufRef.ReadFrom(outReference)
	bufRefStr := bufRef.String()

	bufDumpStr := cmdOutput.String()
	require.Equal(bufRefStr, bufDumpStr)
}

func testFromStdinToFile(t *testing.T) {
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	outFile := filepath.Join(dir, "out.json")

	cmd := bsondumpCommand("--outFile", outFile)

	// Attach a file to stdin of the command.
	inFile, err := os.Open("testdata/sample.bson")
	require.NoError(err)
	cmd.Stdin = inFile

	err = cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	require.NoError(err)
	bufRef := new(bytes.Buffer)
	bufRef.ReadFrom(outReference)
	bufRefStr := bufRef.String()

	// Get the output from a file.
	outDump, err := os.Open(outFile)
	require.NoError(err)
	bufDump := new(bytes.Buffer)
	bufDump.ReadFrom(outDump)
	bufDumpStr := bufDump.String()

	require.Equal(bufRefStr, bufDumpStr)
}

func testFromFileWithNamedArgumentToStdout(t *testing.T) {
	require := require.New(t)
	cmd := bsondumpCommand("--bsonFile", "testdata/sample.bson")

	// Attach a buffer to stdout of the command.
	cmdOutput := &bytes.Buffer{}
	cmd.Stdout = cmdOutput

	err := cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	require.NoError(err)
	bufRef := new(bytes.Buffer)
	bufRef.ReadFrom(outReference)
	bufRefStr := bufRef.String()

	bufDumpStr := cmdOutput.String()
	require.Equal(bufRefStr, bufDumpStr)
}

func testFromFileWithPositionalArgumentToStdout(t *testing.T) {
	require := require.New(t)
	cmd := bsondumpCommand("testdata/sample.bson")

	// Attach a buffer to stdout of command.
	cmdOutput := &bytes.Buffer{}
	cmd.Stdout = cmdOutput

	err := cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	require.NoError(err)
	bufRef := new(bytes.Buffer)
	bufRef.ReadFrom(outReference)
	bufRefStr := bufRef.String()

	bufDumpStr := cmdOutput.String()
	require.Equal(bufRefStr, bufDumpStr)
}

func testFromFileWithNamedArgumentToFile(t *testing.T) {
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	outFile := filepath.Join(dir, "out.json")

	cmd := bsondumpCommand("--outFile", outFile, "--bsonFile", "testdata/sample.bson")

	err := cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	require.NoError(err)
	bufRef := new(bytes.Buffer)
	bufRef.ReadFrom(outReference)
	bufRefStr := bufRef.String()

	// Get the output from a file.
	outDump, err := os.Open(outFile)
	require.NoError(err)
	bufDump := new(bytes.Buffer)
	bufDump.ReadFrom(outDump)
	bufDumpStr := bufDump.String()

	require.Equal(bufRefStr, bufDumpStr)
}

func testFromFileWithPositionalArgumentToFile(t *testing.T) {
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	outFile := filepath.Join(dir, "out.json")

	cmd := bsondumpCommand("--outFile", outFile, "testdata/sample.bson")

	err := cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	require.NoError(err)
	bufRef := new(bytes.Buffer)
	bufRef.ReadFrom(outReference)
	bufRefStr := bufRef.String()

	// Get the output from a file.
	outDump, err := os.Open(outFile)
	require.NoError(err)
	bufDump := new(bytes.Buffer)
	bufDump.ReadFrom(outDump)
	bufDumpStr := bufDump.String()

	require.Equal(bufRefStr, bufDumpStr)
}

func bsondumpCommand(args ...string) *exec.Cmd {
	cmd := []string{"go", "run", filepath.Join("..", "bsondump", "main")}
	cmd = append(cmd, args...)
	return exec.Command(cmd[0], cmd[1:]...)
}
