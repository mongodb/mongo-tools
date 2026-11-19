// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package testopts

import (
	"os"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
)

var (
	UserAdmin              = "uAdmin"
	UserAdminPassword      = "password"
	CreatedUserNameEnv     = "TOOLS_TESTING_AUTH_USERNAME"
	CreatedUserPasswordEnv = "TOOLS_TESTING_AUTH_PASSWORD"
)

// The user that scripts/start-test-cluster.sh creates for the tools under test to authenticate
// as in auth runs: exactly the built-in backup and restore roles, which the documentation
// prescribes as sufficient for mongodump and mongorestore. The values are mirrored in that
// script and are deliberately not configurable so the two cannot drift apart.
const (
	ToolUsername = "backuprestore"
	ToolPassword = "backuprestore"
)

// The user that tests running `mongorestore --oplogReplay` authenticate as, and its password.
// The documentation prescribes a user-defined role with anyAction on anyResource as the minimum
// for oplog replay; testutil.EnsureOplogReplayUser grants this user exactly that role.
const (
	OplogReplayUsername = "oplogreplay"
	OplogReplayPassword = "oplogreplay"
)

// GetAuthOptions returns the credentials of the harness user, which does test setup and
// assertions and holds the __system role the cluster script grants it.
func GetAuthOptions() options.Auth {
	if testtype.HasTestType(testtype.AuthTestType) {
		return options.Auth{
			Username: os.Getenv(CreatedUserNameEnv),
			Password: os.Getenv(CreatedUserPasswordEnv),
			Source:   "admin",
		}
	}

	return options.Auth{}
}

func GetAuthArgs() []string {
	return authArgs(GetAuthOptions())
}

// GetToolAuthOptions returns the credentials the tool under test should authenticate with: the
// backup+restore user, the minimum-privilege identity. Tool runs use these; setup and
// assertions use GetAuthOptions instead.
func GetToolAuthOptions() options.Auth {
	return constantUserAuth(ToolUsername, ToolPassword)
}

// GetOplogReplayAuthOptions returns the credentials for the user that tests running
// `mongorestore --oplogReplay` authenticate as: the anyAction-on-anyResource role the
// documentation prescribes, and nothing else.
func GetOplogReplayAuthOptions() options.Auth {
	return constantUserAuth(OplogReplayUsername, OplogReplayPassword)
}

func constantUserAuth(username, password string) options.Auth {
	if testtype.HasTestType(testtype.AuthTestType) {
		return options.Auth{
			Username: username,
			Password: password,
			Source:   "admin",
		}
	}

	return options.Auth{}
}

func GetToolAuthArgs() []string {
	return authArgs(GetToolAuthOptions())
}

func authArgs(auth options.Auth) []string {
	if !auth.IsSet() {
		return nil
	}

	return []string{
		"--username", auth.Username,
		"--password", auth.Password,
		"--authenticationDatabase", auth.Source,
	}
}
