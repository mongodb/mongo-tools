// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package testutil

import (
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
	"os"
)


func GetAWSOptions() *options.ToolOptions {
	var toolOptions *options.ToolOptions
	if uri := os.Getenv("MONGOD"); uri != "" {
		fakeArgs := []string{"--uri=" + uri}
		toolOptions = options.New("mongodump", "", "", "", true, options.EnabledOptions{URI: true})
		toolOptions.URI.AddKnownURIParameters(options.KnownURIOptionsReadPreference)
		_, err := toolOptions.ParseArgs(fakeArgs)
		if err != nil {
			panic("Could not parse MONGOD environment variable")
		}
	}
	// Limit ToolOptions to test database
	toolOptions.Namespace = &options.Namespace{DB: "aws_test_db"}

	log.SetVerbosity(toolOptions.Verbosity)


	return toolOptions
}
