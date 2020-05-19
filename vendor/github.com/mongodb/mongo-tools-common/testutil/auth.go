// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package testutil

import (
	"os"

	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/testtype"
)

var (
	UserAdmin              = "uAdmin"
	UserAdminPassword      = "password"
	CreatedUserNameEnv     = "TOOLS_TESTING_AUTH_USERNAME"
	CreatedUserPasswordEnv = "TOOLS_TESTING_AUTH_PASSWORD"
)

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

func GetAWSAuthOptions() (*options.ToolOptions, error) {
	opts := &options.ToolOptions{
		Namespace: &options.Namespace{
			DB:         "aws",
			Collection: "test",
		},
		SSL: &options.SSL{},
		Auth: &options.Auth{
			Mechanism: "MONGODB-AWS",
			AWSSessionToken: os.Getenv("SESSION_TOKEN"),
		},
		//Connection: &options.Connection{
		//	Host: "ldaptest.10gen.cc",
		//	Port: "27017",
		//},
		URI: &options.URI{},
	}

	//if runtime.GOOS == "windows" {
	//	opts.Auth.Password = os.Getenv(WinKerberosPwdEnv)
	//	if opts.Auth.Password == "" {
	//		return nil, fmt.Errorf("Need to set %v environment variable to run "+
	//			"kerberos tests on windows", WinKerberosPwdEnv)
	//	}
	//}

	return opts, nil
}

func GetAuthArgs() []string {
	authOpts := GetAuthOptions()
	if authOpts.IsSet() {
		return []string {
			"--username", authOpts.Username,
			"--password", authOpts.Password,
			"--authenticationDatabase", authOpts.Source,
		}
	}
	return nil
}
