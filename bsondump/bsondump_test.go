// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsondump

import (
	"bytes"
	"crypto/rand"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
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
	defer func() {
		require.NoError(inFile.Close())
	}()

	require.NoError(err)
	cmd.Stdin = inFile

	// Attach a buffer to stdout of the command.
	cmdOutput := &bytes.Buffer{}
	cmd.Stdout = cmdOutput

	err = cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	defer func() {
		require.NoError(outReference.Close())
	}()

	require.NoError(err)
	bufRef := new(bytes.Buffer)
	_, err = bufRef.ReadFrom(outReference)
	require.NoError(err)
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
	defer func() {
		require.NoError(inFile.Close())
	}()

	require.NoError(err)
	cmd.Stdin = inFile

	err = cmd.Run()
	require.NoError(err)

	// Get the correct bsondump result from a file to use as a reference.
	outReference, err := os.Open("testdata/sample.json")
	defer func() {
		require.NoError(outReference.Close())
	}()

	require.NoError(err)
	bufRef := new(bytes.Buffer)
	_, err = bufRef.ReadFrom(outReference)
	require.NoError(err)
	bufRefStr := bufRef.String()

	// Get the output from a file.
	outDump, err := os.Open(outFile)
	defer func() {
		require.NoError(outDump.Close())
	}()

	require.NoError(err)
	bufDump := new(bytes.Buffer)
	_, err = bufDump.ReadFrom(outDump)
	require.NoError(err)
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
	defer func() {
		require.NoError(outReference.Close())
	}()

	require.NoError(err)
	bufRef := new(bytes.Buffer)
	_, err = bufRef.ReadFrom(outReference)
	require.NoError(err)
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
	defer func() {
		require.NoError(outReference.Close())
	}()

	require.NoError(err)
	bufRef := new(bytes.Buffer)
	_, err = bufRef.ReadFrom(outReference)
	require.NoError(err)
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
	defer func() {
		require.NoError(outReference.Close())
	}()

	require.NoError(err)
	bufRef := new(bytes.Buffer)
	_, err = bufRef.ReadFrom(outReference)
	require.NoError(err)
	bufRefStr := bufRef.String()

	// Get the output from a file.
	outDump, err := os.Open(outFile)
	defer func() {
		require.NoError(outDump.Close())
	}()

	require.NoError(err)
	bufDump := new(bytes.Buffer)
	_, err = bufDump.ReadFrom(outDump)
	require.NoError(err)
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
	defer func() {
		require.NoError(outReference.Close())
	}()

	require.NoError(err)
	bufRef := new(bytes.Buffer)
	_, err = bufRef.ReadFrom(outReference)
	require.NoError(err)
	bufRefStr := bufRef.String()

	// Get the output from a file.
	outDump, err := os.Open(outFile)
	defer func() {
		require.NoError(outDump.Close())
	}()

	require.NoError(err)
	bufDump := new(bytes.Buffer)
	_, err = bufDump.ReadFrom(outDump)
	require.NoError(err)
	bufDumpStr := bufDump.String()

	require.Equal(bufRefStr, bufDumpStr)
}

func bsondumpCommand(args ...string) *exec.Cmd {
	cmd := []string{"go", "run", filepath.Join("..", "bsondump", "main")}
	cmd = append(cmd, args...)
	return exec.Command(cmd[0], cmd[1:]...)
}

const sixteenKB = 16 * 1024

func TestBsondumpMaxBSONSize(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// 16mb + 16kb
	maxSize := int(16*math.Pow(1024, 2)) + sixteenKB

	t.Run("bsondump with file at exactly max size of 16mb + 16kb", func(t *testing.T) {
		_, err := runBsondumpWithLargeFile(t, maxSize)
		require.NoError(t, err, "no error executing bsondump with large file")
	})
	t.Run("bsondump with file at max size + 1", func(t *testing.T) {
		out, err := runBsondumpWithLargeFile(t, maxSize+1)
		require.Error(t, err, "got error executing bsondump with large file")
		require.Regexp(t, "is larger than maximum of 16793600 bytes", out, "bsondump prints error about file size")
	})
}

func runBsondumpWithLargeFile(t *testing.T, size int) (string, error) {
	require := require.New(t)

	// We need to take the max size and subtract a bunch of things to figure
	// out the size of the binary data to generate.
	//
	// Subtract 16kb for the string field's data.
	//
	// Subtract 4 bytes for the int32 at the head of the document that
	// specifies its size.
	//
	// Subtract 1 byte for the document's trailing NULL.
	//
	// Subtract 2 bytes, one for each byte that specifies the type of our
	// two fields.
	//
	// Subtract 1 byte for the binary field subtype specifier.
	//
	// Subtract the length of each key in the document.
	//
	// Subtract 2 bytes, one for each byte used as the trailing NULL in each
	// of our two keys.
	//
	// Subtract 8 bytes, 4 bytes for each of the int32 values specifying the
	// length of our two fields.
	//
	// Subtract 1 byte for the string field value's trailing NULL.
	binarySize := size - (sixteenKB + 4 + 1 + 2 + 1 + len("name") + len("content") + 2 + 8 + 1)

	buf := make([]byte, binarySize)
	_, err := rand.Read(buf)
	require.NoError(err, "no error creating c. 16mb of random byte data")

	type Large struct {
		Name    string `bson:"name"`
		Content []byte `bson:"content"`
	}
	doc := Large{
		// 16kb
		strings.Repeat("0123456789abcdef", 1024),
		buf,
	}

	marshalled, err := bson.Marshal(doc)
	require.NoError(err, "no error marshalling to BSON")

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	bsonFile := filepath.Join(dir, "in.bson")
	outFile := filepath.Join(dir, "out.json")

	err = os.WriteFile(bsonFile, marshalled, 0644)
	require.NoError(err, "no error writing BSON to %s", bsonFile)

	return runBsondump("--bsonFile", bsonFile, "--outFile", outFile)
}

func runBsondump(args ...string) (string, error) {
	cmd := []string{"go", "run", filepath.Join("..", "bsondump", "main")}
	cmd = append(cmd, args...)

	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	return string(out), err
}
