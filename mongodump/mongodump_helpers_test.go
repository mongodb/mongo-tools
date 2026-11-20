// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testutil"
)

// These are shared by unit and integration tests, so they live in an untagged file.
var (
	// database with test data.
	testDB = "mongodump_test_db"
	// collections in testDB.
	testCollectionNames = []string{"coll1", "coll2", "coll/three"}
)

// simpleMongoDumpInstance builds a mongodump authenticating as the backup user, the
// minimum-privilege identity the documentation prescribes for mongodump. Without the auth test
// type it behaves exactly like the unauthenticated default.
func simpleMongoDumpInstance(t *testing.T) (*MongoDump, error) {
	testutil.EnsureBackupUser(t)
	toolOptions, err := testopts.GetToolOptionsForURIWithAuth(
		os.Getenv(testopts.URIEnvVar),
		testopts.GetBackupAuthOptions(),
	)
	if err != nil {
		return nil, fmt.Errorf("error getting tool options to create a mongodump instance: %w", err)
	}

	// Limit ToolOptions to test database
	toolOptions.Namespace = &options.Namespace{DB: testDB}

	outputOptions := &OutputOptions{
		NumParallelCollections: 1,
	}
	inputOptions := &InputOptions{}

	log.SetVerbosity(toolOptions.Verbosity)

	return &MongoDump{
		ToolOptions:   toolOptions,
		InputOptions:  inputOptions,
		OutputOptions: outputOptions,
	}, nil
}
