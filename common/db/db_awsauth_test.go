// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build awsauth

package db

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/stretchr/testify/require"
)

func TestAWSAuthConnection(t *testing.T) {
	enabled := options.EnabledOptions{URI: true}

	uriBytes, err := os.ReadFile("../testdata/lib/MONGOD_URI")
	if err != nil {
		panic("Could not read MONGOD_URI file")
	}
	uri := string(uriBytes)

	fakeArgs := []string{"--uri=" + uri}
	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err = toolOptions.ParseArgs(fakeArgs)
	if err != nil {
		panic("Could not parse MONGODB_URI file contents")
	}

	_, err = NewSessionProvider(*toolOptions)
	require.NoError(t, err, "connection succeeds")
}
