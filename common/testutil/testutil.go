// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package testutil implements functions for filtering and configuring tests.
package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// GetBareSession returns an mgo.Session from the environment or
// from a default host and port.
func GetBareSession() (*mongo.Client, error) {
	sessionProvider, _, err := GetBareSessionProvider()
	if err != nil {
		return nil, err
	}
	session, err := sessionProvider.GetSession()
	if err != nil {
		return nil, err
	}
	return session, nil
}

// GetBareSessionProvider returns a session provider from the environment or
// from a default host and port.
func GetBareSessionProvider() (*db.SessionProvider, *options.ToolOptions, error) {
	toolOptions, err := GetToolOptions()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"error getting tool options to create a bare session provider: %w",
			err,
		)
	}

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	if err != nil {
		return nil, nil, err
	}

	return sessionProvider, toolOptions, nil
}

const uriEnvVar = "TOOLS_TESTING_MONGOD"

func GetToolOptions() (*options.ToolOptions, error) {
	var toolOptions *options.ToolOptions
	// get ToolOptions from URI or defaults
	if uri := os.Getenv(uriEnvVar); uri != "" {
		parse, err := connstring.ParseAndValidate(uri)
		if err != nil {
			return nil, fmt.Errorf(
				"%#q from the %#q env var is not a valid connection string: %w",
				uri,
				uriEnvVar,
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
				uriEnvVar,
				err,
			)
		}
	} else {
		ssl := GetSSLOptions()
		auth := GetAuthOptions()
		connection := &options.Connection{
			Host: "localhost",
			Port: db.DefaultTestPort,
		}
		toolOptions = &options.ToolOptions{
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

func GetBareArgs() []string {
	args := []string{}

	args = append(args, GetSSLArgs()...)
	args = append(args, GetAuthArgs()...)
	if uri := os.Getenv(uriEnvVar); uri != "" {
		args = append(args, "--uri", uri)
	} else {
		args = append(args, "--host", "localhost", "--port", db.DefaultTestPort)
	}

	return args
}

// GetFCV returns the featureCompatibilityVersion string for an mgo Session
// or the empty string if it can't be found.
func GetFCV(s *mongo.Client) string {
	coll := s.Database("admin").Collection("system.version")
	var result struct {
		Version string
	}
	res := coll.FindOne(context.TODO(), bson.M{"_id": "featureCompatibilityVersion"})
	//nolint:errcheck
	res.Decode(&result)
	return result.Version
}

// CompareFCV compares two strings as dot-delimited tuples of integers.
func CompareFCV(x, y string) (int, error) {
	left, err := dottedStringToSlice(x)
	if err != nil {
		return 0, err
	}
	right, err := dottedStringToSlice(y)
	if err != nil {
		return 0, err
	}

	// Ensure left is the shorter one, flip logic if necessary.
	inverter := 1
	if len(right) < len(left) {
		inverter = -1
		left, right = right, left
	}

	for i := range left {
		switch {
		case left[i] < right[i]:
			return -1 * inverter, nil
		case left[i] > right[i]:
			return 1 * inverter, nil
		}
	}

	// compared equal to length of left. If right is longer, then left is less
	// than right (-1) (modulo the inverter)
	if len(left) < len(right) {
		return -1 * inverter, nil
	}

	return 0, nil
}

func dottedStringToSlice(s string) ([]int, error) {
	parts := make([]int, 0, 2)
	for _, v := range strings.Split(s, ".") {
		i, err := strconv.Atoi(v)
		if err != nil {
			return parts, err
		}
		parts = append(parts, i)
	}
	return parts, nil
}

// MergeOplogStreams combines oplog arrays such that the order of entries is
// random, but order-preserving with respect to each initial stream.
func MergeOplogStreams(input [][]db.Oplog) []db.Oplog {
	// Copy input op arrays so we can destructively shuffle them together
	streams := make([][]db.Oplog, len(input))
	opCount := 0
	for i, v := range input {
		streams[i] = make([]db.Oplog, len(v))
		copy(streams[i], v)
		opCount += len(v)
	}

	ops := make([]db.Oplog, 0, opCount)
	for len(streams) != 0 {
		// randomly pick a stream to add an op
		rand.Shuffle(len(streams), func(i, j int) {
			streams[i], streams[j] = streams[j], streams[i]
		})
		ops = append(ops, streams[0][0])
		// remove the op and its stream if empty
		streams[0] = streams[0][1:]
		if len(streams[0]) == 0 {
			streams = streams[1:]
		}
	}

	return ops
}

// MakeTempDir will attempt to create a temp directory. If it fails it will
// abort the test. It returns two values. The first is the string containing
// the path to the temp directory. The second is a cleanup func that will
// remove the temp directory. You should always call the cleanup func with
// `defer` immedatiately after calling this function:
//
//	dir, cleanup := testutil.MakeTempDir(t)
//	defer cleanup()
//
// If the `TOOLS_TESTING_NO_CLEANUP` env var is not empty, then the cleanup
// function will not delete the directory. This can be useful when
// investigating test failures.
func MakeTempDir(t *testing.T) (string, func()) {
	require := require.New(t)

	dir, err := os.MkdirTemp("", "mongo-tools-test")
	require.NoError(err, "can create temp directory")
	cleanup := func() {
		if os.Getenv("TOOLS_TESTING_NO_CLEANUP") == "" {
			err = os.RemoveAll(dir)
			if err != nil {
				t.Fatalf("Failed to delete temp directory: %v", err)
			}
		}
	}
	return dir, cleanup
}

var atlasDomains = []string{
	".mongo.com",
	".mongodb.net",
	".mongodb-qa.net",
	".mongodb-dev.net",
	".mmscloudteam.com",
	".mmscloudtest.com",
	".mongodbgov.net",
	".mongodbgov-local.net",
	".mongodbgov-dev.net",
	".mongodbgov-qa.net",
}

// SkipForAtlasCluster will skip the test if `TOOLS_TESTING_MONGOD` is an Atlas URI.
func SkipForAtlasCluster(t *testing.T, reason string) {
	uri := os.Getenv(uriEnvVar)
	if uri == "" {
		return
	}

	for _, d := range atlasDomains {
		if strings.Contains(uri, d) {
			t.Skipf(
				"The %#q env var is for an Atlas cluster: %s",
				uriEnvVar,
				reason,
			)
		}
	}
}
