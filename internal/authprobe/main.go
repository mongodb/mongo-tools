// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// The authprobe command waits until the cluster that scripts/start-test-cluster.sh just
// started accepts the bootstrap user's credentials. The runner creates that user directly
// on the cluster's servers, so a mongos in front of them does not know about it until its
// user cache refreshes (about 30 seconds); the first connections through the mongos fail
// authentication until then.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	// The tools connect to 4.2 clusters through this package's init, which lowers the vendored
	// driver's supported wire-version floor; the probe runs that init for the same floor.
	_ "github.com/mongodb/mongo-tools/common/db"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	retryInterval = 2 * time.Second
	deadline      = 90 * time.Second
)

func main() {
	uri := os.Getenv("TOOLS_TESTING_MONGOD")
	username := os.Getenv("TOOLS_TESTING_AUTH_USERNAME")
	password := os.Getenv("TOOLS_TESTING_AUTH_PASSWORD")
	if uri == "" || username == "" || password == "" {
		fmt.Fprintln(
			os.Stderr,
			"authprobe needs TOOLS_TESTING_MONGOD, TOOLS_TESTING_AUTH_USERNAME, and TOOLS_TESTING_AUTH_PASSWORD",
		)
		os.Exit(1)
	}

	stop := time.Now().Add(deadline)
	for {
		client, err := connect(uri, username, password)
		// Connect only fails up front on problems retrying cannot fix, like a malformed URI
		// or a missing TLS file, so those fail immediately instead of burning the deadline.
		if err != nil {
			fmt.Fprintf(os.Stderr, "authprobe cannot connect as %s: %v\n", username, err)
			os.Exit(1)
		}

		err = client.Ping(context.Background(), nil)
		if disconnectErr := client.Disconnect(context.Background()); err == nil {
			err = disconnectErr
		}
		if err == nil {
			return
		}
		if time.Now().After(stop) {
			fmt.Fprintf(os.Stderr, "the %s user never became usable: %v\n", username, err)
			os.Exit(1)
		}
		time.Sleep(retryInterval)
	}
}

func connect(uri, username, password string) (*mongo.Client, error) {
	// A short selection timeout keeps each attempt snappy so the retry loop, not the driver's
	// defaults, decides how long a cluster that is not ready yet takes to give up on.
	return mongo.Connect(
		mopt.Client().
			ApplyURI(uri).
			SetAuth(mopt.Credential{
				Username:   username,
				Password:   password,
				AuthSource: "admin",
			}).
			SetServerSelectionTimeout(retryInterval),
	)
}
