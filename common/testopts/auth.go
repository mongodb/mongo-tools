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

// HarnessAuth returns the credentials of the harness user, the identity test setup and
// assertions connect as. Callers running a tool under test choose a different identity through
// ToolOptions and ToolCLIArgs.
func HarnessAuth() options.Auth {
	if testtype.HasTestType(testtype.AuthTestType) {
		return options.Auth{
			Username: os.Getenv(CreatedUserNameEnv),
			Password: os.Getenv(CreatedUserPasswordEnv),
			Source:   "admin",
		}
	}

	return options.Auth{}
}

// AuthArgs returns the command-line flags for the given credentials, for the suites that build
// a tool's arguments by hand.
func AuthArgs(auth options.Auth) []string {
	if !auth.IsSet() {
		return nil
	}

	return []string{
		"--username", auth.Username,
		"--password", auth.Password,
		"--authenticationDatabase", auth.Source,
	}
}
