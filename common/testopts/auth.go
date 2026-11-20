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

// The users that the tools under test authenticate as in auth runs: one per tool, holding
// exactly the role the documentation prescribes as sufficient for it — backup for mongodump,
// restore for mongorestore — so a dump run never holds restore privileges and the other way
// around. The values are mirrored in common/testutil, which creates these users, and are
// deliberately not configurable so the two cannot drift apart.
const (
	BackupUsername  = "backup"
	BackupPassword  = "backup"
	RestoreUsername = "restore"
	RestorePassword = "restore"
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

// AuthArgs returns the command-line flags carrying the given credentials, for the suites that
// build a tool's arguments by hand.
func AuthArgs(auth options.Auth) []string {
	return authArgs(auth)
}

// GetBackupAuthOptions returns the credentials mongodump runs should authenticate with: the
// backup user, the minimum-privilege identity for mongodump. Setup and assertions use
// GetAuthOptions instead.
func GetBackupAuthOptions() options.Auth {
	return constantUserAuth(BackupUsername, BackupPassword)
}

// GetRestoreAuthOptions returns the credentials mongorestore runs should authenticate with:
// the restore user, the minimum-privilege identity for mongorestore.
func GetRestoreAuthOptions() options.Auth {
	return constantUserAuth(RestoreUsername, RestorePassword)
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
