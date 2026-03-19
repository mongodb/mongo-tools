// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsondump

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
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

// TestBsondumpAllTypesDebug verifies that bsondump --type=debug outputs the correct BSON type
// numbers for all non-deprecated BSON types (from all_types.js).
func TestBsondumpAllTypesDebug(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	out, err := runBsondump("--type=debug", "testdata/all_types.bson")
	require.NoError(t, err, "bsondump should exit successfully with --type=debug")

	assert.Equal(
		t,
		22,
		strings.Count(out, "--- new object ---"),
		"should find all 22 documents in all_types.bson",
	)

	for _, typeNum := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 17, 18, -1, 127} {
		re := regexp.MustCompile(fmt.Sprintf(`type:\s+%d`, typeNum))
		assert.Regexp(t, re, out, "expected type %d in debug output", typeNum)
	}
}

// TestBsondumpAllTypesJSON verifies that bsondump --type=json correctly serializes all
// non-deprecated BSON types to Extended JSON (from all_types_json.js).
func TestBsondumpAllTypesJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	oid, err := bson.ObjectIDFromHex("507f1f77bcf86cd799439011")
	require.NoError(t, err)
	dec, err := bson.ParseDecimal128("1.2E+10")
	require.NoError(t, err)

	doc := bson.D{
		{"double", 2.0},
		{"string", "hi"},
		{"doc", bson.D{{"x", int32(1)}}},
		{"arr", bson.A{int32(1), int32(2)}},
		{"binary", bson.Binary{Subtype: 0x00, Data: []byte{0xff, 0xff}}},
		{"oid", oid},
		{"bool", true},
		{"date", bson.DateTime(978312200000)},
		{"code", bson.CodeWithScope{Code: "hi", Scope: bson.D{{"x", int32(1)}}}},
		{"ts", bson.Timestamp{T: 1, I: 2}},
		{"int32", int32(5)},
		{"int64", int64(6)},
		{"dec", dec},
		{"minkey", bson.MinKey{}},
		{"maxkey", bson.MaxKey{}},
		{"regex", bson.Regex{Pattern: "^abc", Options: "imx"}},
		{"symbol", bson.Symbol("i am a symbol")},
		{"undefined", bson.Undefined{}},
		{"dbpointer", bson.DBPointer{DB: "some.namespace", Pointer: oid}},
		{"null", bson.Null{}},
	}

	bsonData, err := bson.Marshal(doc)
	require.NoError(t, err)
	tmpFile, err := os.CreateTemp(t.TempDir(), "all_types_*.bson")
	require.NoError(t, err)
	_, err = tmpFile.Write(bsonData)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	expectedJSON, err := bson.MarshalExtJSON(doc, true, false)
	require.NoError(t, err)

	out, err := runBsondump("--type=json", tmpFile.Name())
	require.NoError(t, err, "bsondump should exit successfully with 0")

	assert.Contains(
		t,
		out,
		"1 objects found",
		"should print out all top-level documents from the test data",
	)

	var jsonLine string
	for line := range strings.Lines(out) {
		if strings.Contains(line, "$oid") {
			jsonLine = line
			break
		}
	}
	require.NotEmpty(t, jsonLine, "should find a JSON output line containing Extended JSON")
	assert.JSONEq(t, string(expectedJSON), jsonLine)
}

// TestBsondumpDeepNested verifies bsondump handles deeply nested BSON
// documents without error in both JSON and debug modes (from deep_nested.js).
func TestBsondumpDeepNested(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_, err := runBsondump("--type=json", "testdata/deep_nested.bson")
	assert.NoError(t, err, "bsondump should handle deeply nested documents in JSON mode")

	_, err = runBsondump("--type=debug", "testdata/deep_nested.bson")
	assert.NoError(t, err, "bsondump should handle deeply nested documents in debug mode")
}

// TestBsondumpBadFiles verifies bsondump error handling for malformed BSON
// input with and without --objcheck (from bad_files.js).
func TestBsondumpBadFiles(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	badFiles := []string{
		"testdata/bad_cstring.bson",
		"testdata/bad_type.bson",
		"testdata/invalid_field_name.bson",
		"testdata/partial_file.bson",
		"testdata/random_bytes.bson",
	}
	for _, f := range badFiles {
		_, err := runBsondump("--objcheck", f)
		assert.Error(t, err, "--objcheck %s should exit with error", f)

		_, err = runBsondump("--objcheck", "--type=debug", f)
		assert.Error(t, err, "--objcheck --type=debug %s should exit with error", f)
	}

	_, err := runBsondump("--objcheck", "testdata/broken_array.bson")
	assert.NoError(t, err, "--objcheck broken_array.bson should succeed")

	_, err = runBsondump("--objcheck", "--type=debug", "testdata/broken_array.bson")
	assert.NoError(t, err, "--objcheck --type=debug broken_array.bson should succeed")

	out, err := runBsondump("testdata/bad_cstring.bson")
	assert.NoError(t, err, "bad_cstring.bson without --objcheck should not error")
	assert.Contains(
		t,
		out,
		"unable to dump document",
		"bad_cstring.bson should report a corrupted document in output",
	)
}

// TestBsondumpOptionValidation verifies bsondump accepts valid options and
// rejects invalid ones (from bsondump_options.js).
func TestBsondumpOptionValidation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("invalid --type fails", func(t *testing.T) {
		_, err := runBsondump("--type=fake", "testdata/sample.bson")
		assert.Error(t, err)
	})
	t.Run("nonexistent file fails", func(t *testing.T) {
		_, err := runBsondump("testdata/does_not_exist.bson")
		assert.Error(t, err)
	})
	t.Run("--noobjcheck fails", func(t *testing.T) {
		_, err := runBsondump("--noobjcheck", "testdata/sample.bson")
		assert.Error(t, err)
	})
	t.Run("--collection fails", func(t *testing.T) {
		_, err := runBsondump("--collection", "testdata/sample.bson")
		assert.Error(t, err)
	})
	t.Run("multiple positional args fails", func(t *testing.T) {
		_, err := runBsondump("testdata/sample.bson", "testdata/sample.bson")
		assert.Error(t, err)
	})
	t.Run("--bsonFile with extra positional arg fails", func(t *testing.T) {
		_, err := runBsondump("--bsonFile", "testdata/sample.bson", "testdata/sample.bson")
		assert.Error(t, err)
	})
	t.Run("-vvvv succeeds", func(t *testing.T) {
		_, err := runBsondump("-vvvv", "testdata/sample.bson")
		assert.NoError(t, err)
	})
	t.Run("--verbose succeeds", func(t *testing.T) {
		_, err := runBsondump("--verbose", "testdata/sample.bson")
		assert.NoError(t, err)
	})
	t.Run("--quiet suppresses status but still outputs data", func(t *testing.T) {
		out, err := runBsondump("--quiet", "testdata/sample.bson")
		assert.NoError(t, err)
		assert.Contains(t, out, "I am a string",
			"JSON content should still be output with --quiet")
		assert.NotContains(t, out, "objects found",
			"status line should be suppressed by --quiet")
	})
	t.Run("--help succeeds and prints usage", func(t *testing.T) {
		out, err := runBsondump("--help")
		assert.NoError(t, err)
		assert.Contains(t, out, "Usage")
	})
	t.Run("--version succeeds and prints version", func(t *testing.T) {
		out, err := runBsondump("--version")
		assert.NoError(t, err)
		assert.Contains(t, out, "version")
	})
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
	maxSize := (16 * 1024 * 1024) + sixteenKB

	t.Run("bsondump with file at exactly max size of 16mb + 16kb", func(t *testing.T) {
		_, err := runBsondumpWithLargeFile(t, maxSize)
		require.NoError(t, err, "no error executing bsondump with large file")
	})
	t.Run("bsondump with file at max size + 1", func(t *testing.T) {
		out, err := runBsondumpWithLargeFile(t, maxSize+1)
		require.Error(t, err, "got error executing bsondump with large file")
		require.Regexp(
			t,
			"is larger than maximum of 16793600 bytes",
			out,
			"bsondump prints error about file size",
		)
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

	marshaled, err := bson.Marshal(doc)
	require.NoError(err, "no error marshaling to BSON")

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	bsonFile := filepath.Join(dir, "in.bson")
	outFile := filepath.Join(dir, "out.json")

	err = os.WriteFile(bsonFile, marshaled, 0644)
	require.NoError(err, "no error writing BSON to %s", bsonFile)

	return runBsondump("--bsonFile", bsonFile, "--outFile", outFile)
}

func runBsondump(args ...string) (string, error) {
	cmd := []string{"go", "run", filepath.Join("..", "bsondump", "main")}
	cmd = append(cmd, args...)

	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	return string(out), err
}
