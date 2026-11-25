// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package testopts

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthOptionsFollowTestType(t *testing.T) {
	t.Run("without the auth test type", func(t *testing.T) {
		t.Setenv(testtype.AuthTestType, "")
		t.Setenv(CreatedUserNameEnv, "")
		t.Setenv(CreatedUserPasswordEnv, "")

		harness := HarnessAuth()
		assert.False(
			t,
			harness.IsSet(),
			"the harness user is unset without the auth test type",
		)
		backup := BackupAuth()
		assert.False(
			t,
			backup.IsSet(),
			"the backup user is unset without the auth test type",
		)
		restore := RestoreAuth()
		assert.False(
			t,
			restore.IsSet(),
			"the restore user is unset without the auth test type",
		)
		oplogReplay := OplogReplayAuth()
		assert.False(
			t,
			oplogReplay.IsSet(),
			"the oplog-replay user is unset without the auth test type",
		)
	})

	t.Run("with the auth test type", func(t *testing.T) {
		t.Setenv(testtype.AuthTestType, "true")
		t.Setenv(CreatedUserNameEnv, "harness-user")
		t.Setenv(CreatedUserPasswordEnv, "harness-pass")

		harness := HarnessAuth()
		assert.Equal(
			t,
			"harness-user",
			harness.Username,
			"the harness user comes from the environment",
		)
		assert.Equal(
			t,
			"harness-pass",
			harness.Password,
			"the harness password comes from the environment",
		)

		backup := BackupAuth()
		assert.Equal(
			t,
			BackupUsername,
			backup.Username,
			"the backup user is the user mongodump runs as",
		)
		assert.Equal(
			t,
			BackupPassword,
			backup.Password,
			"the backup user's password is the fixed constant",
		)

		restore := RestoreAuth()
		assert.Equal(
			t,
			RestoreUsername,
			restore.Username,
			"the restore user is the user mongorestore runs as",
		)
		assert.Equal(
			t,
			RestorePassword,
			restore.Password,
			"the restore user's password is the fixed constant",
		)

		oplogReplay := OplogReplayAuth()
		assert.Equal(
			t,
			OplogReplayUsername,
			oplogReplay.Username,
			"the oplog-replay user is the anyAction-on-anyResource user",
		)
		assert.Equal(
			t,
			OplogReplayPassword,
			oplogReplay.Password,
			"the oplog-replay user's password is the fixed constant",
		)
	})
}

func TestToolOptions(t *testing.T) {
	// The connection string the test cluster exports names no credentials; the identity comes
	// from the auth parameter.
	const uri = "mongodb://127.0.0.1:33333/?authSource=admin"

	opts, err := ToolOptions(uri, options.Auth{
		Username: "someuser",
		Password: "somepass",
		Source:   "admin",
	})
	require.NoError(t, err, "building options for a credential-less URI")

	assert.Equal(t, "someuser", opts.Username, "the chosen identity is set on the options")
	assert.Equal(
		t,
		"somepass",
		opts.Password,
		"the chosen identity's password is set on the options",
	)
}

func TestToolCLIArgs(t *testing.T) {
	const uri = "mongodb://127.0.0.1:33333/?authSource=admin"

	auth := options.Auth{Username: "someuser", Password: "somepass", Source: "admin"}

	args := ToolCLIArgs(uri, auth)
	assert.Contains(t, args, "--username", "the chosen identity is passed as a flag")
	assert.Contains(t, args, "someuser", "the chosen username is passed as a flag value")
	assert.Contains(t, args, "--uri", "the URI is passed through")

	noAuth := ToolCLIArgs(uri, options.Auth{})
	assert.NotContains(t, noAuth, "--username", "no identity flags without credentials")
}
