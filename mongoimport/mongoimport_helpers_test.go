// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"fmt"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
)

// These are shared by unit and integration tests, so they live in an untagged file.
const (
	testDb         = "db"
	testCollection = "c"
	mioSoeFile     = "testdata/10k1dup10k.json"
)

func newOptions() Options {
	return Options{
		ToolOptions: getBasicToolOptions(),
		InputOptions: &InputOptions{
			ParseGrace: "stop",
		},
		IngestOptions: &IngestOptions{},
	}
}

// getBasicToolOptions returns a test helper to instantiate the session provider
// for calls to StreamDocument.
func getBasicToolOptions() *options.ToolOptions {
	toolOptions, err := testopts.GetToolOptions()
	if err != nil {
		panic(fmt.Sprintf("could not get tool options: %v", err))
	}
	toolOptions.Namespace.DB = testDb
	toolOptions.Namespace.Collection = testCollection

	return toolOptions
}
