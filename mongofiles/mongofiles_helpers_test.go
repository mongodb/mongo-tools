// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"fmt"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// These are shared by unit and integration tests, so they live in an untagged file.
var (
	testDB = "mongofiles_test_db"

	ssl         = testopts.GetSSLOptions()
	toolOptions = mustGetToolOptions()
	testFiles   = map[string]bson.ObjectID{
		"testfile1": bson.NewObjectID(),
		"testfile2": bson.NewObjectID(),
		"testfile3": bson.NewObjectID(),
		"testfile4": bson.NewObjectID(),
	}
)

func mustGetToolOptions() *options.ToolOptions {
	toolOptions, err := testopts.GetToolOptions()
	if err != nil {
		panic(fmt.Sprintf("could not get tool options: %v", err))
	}

	return toolOptions
}
