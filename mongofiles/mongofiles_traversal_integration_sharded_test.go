// Copyright (C) MongoDB, Inc. 2026-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package mongofiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gridFSContent  = "from gridfs"
	outsideContent = "outside"
)

// TestGetWritesThroughSymlinks covers how 'get' and 'get_regex' treat a
// symlink that already exists in the current directory under the name of the
// GridFS file being downloaded.
func TestGetWritesThroughSymlinks(t *testing.T) {
	t.Run("get_regex refuses a symlink that escapes the current directory", func(t *testing.T) {
		cwd := setUpSymlinkTest(t, "evil.txt")
		outside := writeOutsideFile(t)
		require.NoError(t, os.Symlink(outside, filepath.Join(cwd, "evil.txt")))

		_, err := runMongoFiles(t, getRegexCommand("evil.txt"))
		require.Error(t, err, "get_regex fails instead of following the symlink")
		assert.Contains(t, err.Error(), "--allowUnsafeTraversal", "error suggests the opt-in flag")
		assertFileContent(
			t,
			outside,
			outsideContent,
			"file outside the current directory is untouched",
		)
	})

	t.Run("get_regex follows a relative symlink inside the current directory", func(t *testing.T) {
		cwd := setUpSymlinkTest(t, "report.txt")
		require.NoError(t, os.Mkdir(filepath.Join(cwd, "data"), 0o755))
		require.NoError(t, os.Symlink(
			filepath.Join("data", "report.txt"),
			filepath.Join(cwd, "report.txt"),
		))

		_, err := runMongoFiles(t, getRegexCommand("report.txt"))
		require.NoError(t, err, "get_regex follows a relative symlink that stays inside")
		assertFileContent(t, filepath.Join(cwd, "data", "report.txt"), gridFSContent,
			"GridFS content is written to the symlink target")
	})

	t.Run(
		"get_regex refuses an absolute symlink even inside the current directory",
		func(t *testing.T) {
			cwd := setUpSymlinkTest(t, "report.txt")
			require.NoError(t, os.Mkdir(filepath.Join(cwd, "data"), 0o755))
			target := filepath.Join(cwd, "data", "report.txt")
			require.NoError(t, os.Symlink(target, filepath.Join(cwd, "report.txt")))

			_, err := runMongoFiles(t, getRegexCommand("report.txt"))
			require.Error(t, err, "os.Root rejects absolute symlink targets")
			assert.NoFileExists(t, target, "symlink target is not created")
		},
	)

	t.Run("get_regex follows an escaping symlink with --allowUnsafeTraversal", func(t *testing.T) {
		cwd := setUpSymlinkTest(t, "evil.txt")
		outside := writeOutsideFile(t)
		require.NoError(t, os.Symlink(outside, filepath.Join(cwd, "evil.txt")))

		mf := getRegexCommand("evil.txt")
		mf.StorageOptions.AllowUnsafeTraversal = true
		_, err := runMongoFiles(t, mf)
		require.NoError(t, err, "get_regex follows the symlink when traversal is allowed")
		assertFileContent(t, outside, gridFSContent, "symlink target is overwritten")
	})

	t.Run("get follows a symlink for a name the user typed", func(t *testing.T) {
		cwd := setUpSymlinkTest(t, "evil.txt")
		outside := writeOutsideFile(t)
		require.NoError(t, os.Symlink(outside, filepath.Join(cwd, "evil.txt")))

		mf := newMongoFiles("get")
		mf.FileNameList = []string{"evil.txt"}
		_, err := runMongoFiles(t, mf)
		require.NoError(t, err, "get trusts a file name the user chose")
		assertFileContent(t, outside, gridFSContent, "symlink target is overwritten")
	})
}

// setUpSymlinkTest changes into a fresh directory and stores a GridFS file
// named name, leaving no local file of that name behind.
func setUpSymlinkTest(t *testing.T, name string) string {
	require.NoError(t, tearDownGridFSTestData(t))
	t.Cleanup(func() { _ = tearDownGridFSTestData(t) })

	cwd, err := filepath.Abs(t.TempDir())
	require.NoError(t, err)
	t.Chdir(cwd)

	require.NoError(t, os.WriteFile(name, []byte(gridFSContent), 0o644))
	mf := newMongoFiles("put")
	mf.FileName = name
	_, err = runMongoFiles(t, mf)
	require.NoError(t, err, "put stores the GridFS file")
	require.NoError(t, os.Remove(name))

	return cwd
}

func writeOutsideFile(t *testing.T) string {
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte(outsideContent), 0o644))
	return outside
}

func newMongoFiles(command string) *MongoFiles {
	return &MongoFiles{
		ToolOptions:  toolOptions,
		InputOptions: &InputOptions{},
		StorageOptions: &StorageOptions{
			GridFSPrefix:   "fs",
			DB:             testDB,
			OverwriteLocal: true,
		},
		Command: command,
	}
}

func getRegexCommand(regex string) *MongoFiles {
	mf := newMongoFiles("get_regex")
	mf.FileNameRegex = regex
	return mf
}

func runMongoFiles(t *testing.T, mf *MongoFiles) (string, error) {
	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	require.NoError(t, err)
	t.Cleanup(sessionProvider.Close)
	mf.SessionProvider = sessionProvider
	return mf.Run(false)
}

func assertFileContent(t *testing.T, path, want, msg string) {
	got, err := os.ReadFile(path)
	require.NoError(t, err, msg)
	assert.Equal(t, want, string(got), msg)
}
