// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestGetToStdout(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	emptyGridFSClient(t)
	localFile := loremIpsumFiles[0]

	runPut(t, StorageOptions{}, localFile)

	mf := mongoFilesWithStorageOptions(t, "get", StorageOptions{LocalFileName: "-"})
	mf.FileName = localFile

	captured := captureStdout(t, func() {
		_, err := mf.Run(false)
		require.NoError(t, err, "a get with --local - succeeds")
	})

	want, err := os.ReadFile(localFile)
	require.NoError(t, err, "can read %#q", localFile)

	assert.Equal(t, want, captured, "the file's contents are written to stdout")
}

func captureStdout(t *testing.T, run func()) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(path)
	require.NoError(t, err, "can create the file standing in for stdout")

	// Restored with defer so that a failed assertion inside run, which exits the
	// goroutine, cannot leave the rest of the package writing to this file.
	saved := os.Stdout
	os.Stdout = file
	defer func() {
		os.Stdout = saved
	}()

	run()

	require.NoError(t, file.Close(), "can close the file standing in for stdout")

	captured, err := os.ReadFile(path)
	require.NoError(t, err, "can read what was written to stdout")

	return captured
}

func TestGetByIDMatchesGetByName(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client := emptyGridFSClient(t)
	localFile := loremIpsumFiles[0]

	runPut(t, StorageOptions{}, localFile)

	tmpDir := t.TempDir()
	byName := filepath.Join(tmpDir, "by-name.txt")
	byID := filepath.Join(tmpDir, "by-id.txt")

	mf := mongoFilesWithStorageOptions(t, "get", StorageOptions{LocalFileName: byName})
	mf.FileName = localFile
	_, err := mf.Run(false)
	require.NoError(t, err, "a get by file name succeeds")

	mf = mongoFilesWithStorageOptions(t, "get_id", StorageOptions{LocalFileName: byID})
	mf.Id = extendedJSONID(t, gridFSFile(t, client, localFile).ID)
	_, err = mf.Run(false)
	require.NoError(t, err, "a get_id with the file's _id as extended JSON succeeds")

	assertFilesIdentical(t, localFile, byName)
	assertFilesIdentical(t, localFile, byID)
}

func extendedJSONID(t *testing.T, id any) string {
	t.Helper()

	oid, ok := id.(bson.ObjectID)
	require.True(t, ok, "the GridFS file's _id is an ObjectID")

	return fmt.Sprintf(`{"$oid":%q}`, oid.Hex())
}

func TestSearchCommand(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	emptyGridFSClient(t)

	for _, name := range loremIpsumFiles {
		runPut(t, StorageOptions{}, name)
	}

	for _, pattern := range []string{"lorem", "multi_args", `\.txt`, "."} {
		assert.Equal(
			t,
			loremIpsumFiles,
			searchResults(t, pattern),
			"a search for %#q returns every file",
			pattern,
		)
	}

	for _, pattern := range []string{"random", "always", "filer"} {
		assert.Empty(
			t,
			searchResults(t, pattern),
			"a search for %#q returns nothing",
			pattern,
		)
	}

	// The names are quoted because search treats its argument as a regex, and on
	// Windows they contain backslash separators.
	for _, name := range loremIpsumFiles {
		assert.Equal(
			t,
			[]string{name},
			searchResults(t, regexp.QuoteMeta(name)),
			"a search for %#q returns that file alone",
			name,
		)
	}
}

func searchResults(t *testing.T, pattern string) []string {
	t.Helper()

	mf := mongoFilesWithStorageOptions(t, "search", StorageOptions{})
	mf.FileName = pattern

	output, err := mf.Run(false)
	require.NoError(t, err, "can search for %#q", pattern)

	if output == "" {
		return nil
	}

	names := make([]string, 0, len(loremIpsumFiles))
	for name := range getFilesAndBytesFromLines(cleanAndTokenizeTestOutput(output)) {
		names = append(names, name)
	}
	slices.Sort(names)

	return names
}

// TestRejectsUnknownFlag covers an unknown flag. The other way of naming
// something mongofiles does not have, an unknown subcommand, is the "nonsensical
// command" case of TestValidArguments, which reaches ValidateCommand without
// connecting.
func TestRejectsUnknownFlag(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_, err := ParseOptions([]string{"--invalid", "33333", "put", "file"}, "", "")
	require.ErrorContains(
		t,
		err,
		"unknown option `invalid`",
		"an unrecognized flag is rejected",
	)
}
