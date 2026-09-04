// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package testopts builds the connection options and command-line arguments that point a test at
// the server under test. It is separate from testutil, which it would otherwise belong to, because
// common/db's own tests need these helpers and testutil imports common/db.
package testopts

import (
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

const (
	// DefaultTestPort is the port the test suites expect a mongod on unless the environment overrides it
	// through URIEnvVar.
	DefaultTestPort = "33333"

	// URIEnvVar names the env var that points the test suites at the server under test, overriding
	// localhost:DefaultTestPort.
	URIEnvVar = "TOOLS_TESTING_MONGOD"

	// URIEnvVar2 names the env var that points the test suites at a second, distinct server under
	// test.
	//
	// Tests that need a second cluster (e.g. a dump on one cluster restored into another) read it
	// through SecondURI, and are skipped when it is empty.
	URIEnvVar2 = "TOOLS_TESTING_MONGOD2"
)

// SecondURI returns the connection string for the second server under test, or the empty string
// when none is configured.
func SecondURI() string {
	return os.Getenv(URIEnvVar2)
}

func GetToolOptions() (*options.ToolOptions, error) {
	return GetToolOptionsForURI(os.Getenv(URIEnvVar))
}

// GetToolOptionsForURI builds ToolOptions pointing at the given URI, or at the default
// localhost:DefaultTestPort when uri is empty.
func GetToolOptionsForURI(uri string) (*options.ToolOptions, error) {
	var toolOptions *options.ToolOptions
	// get ToolOptions from URI or defaults
	if uri != "" {
		parse, err := connstring.ParseAndValidate(uri)
		if err != nil {
			return nil, fmt.Errorf(
				"%#q from the %#q env var is not a valid connection string: %w",
				uri,
				URIEnvVar,
				err,
			)
		}

		fakeArgs := []string{"--uri=" + uri}
		opts := options.EnabledOptions{Auth: parse.UsernameSet, URI: true}
		toolOptions = options.New("mongodump", "", "", "", true, opts)

		_, err = toolOptions.ParseArgs(fakeArgs)
		if err != nil {
			return nil, fmt.Errorf(
				"could not create toolOptions with %#q from the %#q env var: %w",
				uri,
				URIEnvVar,
				err,
			)
		}

		// ParseArgs does not set this, but tools like mongoimport and mongorestore
		// dereference it without a nil check.
		toolOptions.WriteConcern = wcwrapper.Majority()
	} else {
		ssl := GetSSLOptions()
		auth := GetAuthOptions()
		connection := &options.Connection{
			Host: "localhost",
			Port: DefaultTestPort,
		}
		toolOptions = &options.ToolOptions{
			General:      &options.General{},
			SSL:          &ssl,
			Connection:   connection,
			Auth:         &auth,
			Verbosity:    &options.Verbosity{},
			URI:          &options.URI{},
			Namespace:    &options.Namespace{},
			WriteConcern: wcwrapper.Majority(),
		}
	}

	err := toolOptions.NormalizeOptionsAndURI()
	if err != nil {
		return nil, err
	}

	return toolOptions, nil
}

// MustGetToolOptions is GetToolOptions for the tests that have nothing left to do if the options
// cannot be built.
func MustGetToolOptions(t *testing.T) options.ToolOptions {
	t.Helper()

	opts, err := GetToolOptions()
	require.NoError(t, err, "getting the tool options for the server under test")

	return *opts
}

func GetBareArgs() []string {
	return GetBareArgsForURI(os.Getenv(URIEnvVar))
}

// GetBareArgsForURI returns the command-line args that point a tool at the given URI, or at the
// default localhost:DefaultTestPort when uri is empty.
func GetBareArgsForURI(uri string) []string {
	args := []string{}

	args = append(args, GetSSLArgs()...)
	args = append(args, GetAuthArgs()...)
	if uri != "" {
		args = append(args, "--uri", uri)
	} else {
		args = append(args, "--host", "localhost", "--port", DefaultTestPort)
	}

	return args
}
