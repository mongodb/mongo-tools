// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package testutil implements functions for filtering and configuring tests.
package testutil

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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
	var toolOptions *options.ToolOptions

	// get ToolOptions from URI or defaults
	if uri := os.Getenv("TOOLS_TESTING_MONGOD"); uri != "" {
		fakeArgs := []string{"--uri=" + uri}
		toolOptions = options.New("mongodump", "", "", "", true, options.EnabledOptions{URI: true})
		_, err := toolOptions.ParseArgs(fakeArgs)
		if err != nil {
			panic(fmt.Sprintf("Could not parse TOOLS_TESTING_MONGOD environment variable: %v", err))
		}
	} else {
		ssl := GetSSLOptions()
		auth := GetAuthOptions()
		connection := &options.Connection{
			Host: "localhost",
			Port: db.DefaultTestPort,
		}
		toolOptions = &options.ToolOptions{
			SSL:        &ssl,
			Connection: connection,
			Auth:       &auth,
			Verbosity:  &options.Verbosity{},
			URI:        &options.URI{},
			Namespace:  &options.Namespace{},
		}
	}
	err := toolOptions.NormalizeOptionsAndURI()
	if err != nil {
		return nil, nil, err
	}
	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	if err != nil {
		return nil, nil, err
	}
	return sessionProvider, toolOptions, nil
}

func GetBareArgs() []string {
	args := []string{}

	args = append(args, GetSSLArgs()...)
	args = append(args, GetAuthArgs()...)
	args = append(args, "--host", "localhost", "--port", db.DefaultTestPort)

	return args
}

// GetFCV returns the featureCompatibilityVersion string for an mgo Session
// or the empty string if it can't be found.
func GetFCV(s *mongo.Client) string {
	coll := s.Database("admin").Collection("system.version")
	var result struct {
		Version string
	}
	res := coll.FindOne(nil, bson.M{"_id": "featureCompatibilityVersion"})
	res.Decode(&result)
	return result.Version
}

// CompareFCV compares two strings as dot-delimited tuples of integers
func CompareFCV(x, y string) (int, error) {
	left, err := dottedStringToSlice(x)
	if err != nil {
		return 0, err
	}
	right, err := dottedStringToSlice(y)
	if err != nil {
		return 0, err
	}

	// Ensure left is the shorter one, flip logic if necesary
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

	dir, err := ioutil.TempDir("", "mongo-tools-test")
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
