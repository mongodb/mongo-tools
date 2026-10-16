// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongofiles

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test that it works whenever valid arguments are passed in and that
// it barfs whenever invalid ones are passed.
func TestValidArguments(t *testing.T) {
	mf := simpleMockMongoFilesInstanceWithFilename("search", "file")
	t.Run("no args", func(t *testing.T) {
		args := []string{}
		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(t, err, "no command specified")
	})

	t.Run("non-URI positional args", func(t *testing.T) {
		for _, command := range []string{"list", "delete", "search", "get_id", "delete_id"} {
			args := []string{command, "arg1", "arg2"}
			err := mf.ValidateCommand(args)
			require.Error(t, err)
			assert.EqualError(
				t,
				err,
				"too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
				"%v: too many args",
				args,
			)
		}
	})

	t.Run("put_id with too many args", func(t *testing.T) {
		args := []string{"put_id", "arg1", "arg2", "arg3"}
		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(
			t,
			err,
			"too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
			"%v: too many args",
			args,
		)
	})

	t.Run("put_id with too few args", func(t *testing.T) {
		args := []string{"put_id", "arg1"}
		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("%#q argument(s) missing", "put_id"))
	})

	t.Run("list with no args", func(t *testing.T) {
		args := []string{"list"}
		require.NoError(t, mf.ValidateCommand(args))
		assert.Equal(t, "", mf.StorageOptions.LocalFileName)
	})

	t.Run("get with multiple args", func(t *testing.T) {
		args := []string{"get", "foo", "bar", "baz"}
		require.NoError(t, mf.ValidateCommand(args))
		assert.Equal(t, []string{"foo", "bar", "baz"}, mf.FileNameList)
	})

	t.Run("put with multiple args", func(t *testing.T) {
		args := []string{"put", "foo", "bar", "baz"}
		require.NoError(t, mf.ValidateCommand(args))
		assert.Equal(t, []string{"foo", "bar", "baz"}, mf.FileNameList)
	})

	t.Run("too few args", func(t *testing.T) {
		for _, command := range []string{"get", "put", "delete", "search", "get_id", "delete_id"} {
			args := []string{command}
			err := mf.ValidateCommand(args)
			require.Error(t, err)
			assert.EqualError(
				t,
				err,
				fmt.Sprintf("%#q argument missing", command),
				"%s with no additional argument",
				command,
			)
		}
	})

	t.Run("nonsensical command", func(t *testing.T) {
		args := []string{"commandnonexistent"}

		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(
			t,
			err,
			fmt.Sprintf(
				"%#q is not a valid command (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
				args[0],
			),
		)
	})
}

// simpleMockMongoFilesInstanceWithFilename gets an instance of MongoFiles with no underlying SessionProvider.
// Use this for tests that don't communicate with the server (e.g. options parsing tests).
func simpleMockMongoFilesInstanceWithFilename(command, fname string) *MongoFiles {
	return &MongoFiles{
		ToolOptions:    toolOptions,
		InputOptions:   &InputOptions{},
		StorageOptions: &StorageOptions{GridFSPrefix: "fs", DB: testDB},
		Command:        command,
		FileName:       fname,
	}
}
