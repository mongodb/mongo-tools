// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package dsc detects whether a connected server uses disaggregated storage (DSC). The detector
// works on a *mongo.Client so it can be shared between the production tools and the test suite.
package dsc

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
)

// A disaggregated server reports this as its WiredTiger storage type, where a normal one reports
// "file".
const disaggregatedStorageType = "layered"

// IsDisaggregatedStorage reports whether the connected server uses disaggregated storage (DSC).
//
// The storage type comes from $collStats rather than from a server parameter because the parameter
// naming DSC only exists on mongod: through a mongos it looks exactly like a server too old to
// know it, and the caller would run instead of failing fast.
func IsDisaggregatedStorage(ctx context.Context, client *mongo.Client) (bool, error) {
	opts := mopt.Database().SetReadConcern(readconcern.Majority())
	coll := client.Database("admin", opts).Collection("system.version")

	pipeline := mongo.Pipeline{
		bson.D{{"$collStats", bson.D{{"storageStats", bson.D{}}}}},
		bson.D{{"$project", bson.D{{"type", "$storageStats.wiredTiger.type"}}}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return false, fmt.Errorf("fetching the cluster storage type: %w", err)
	}

	var stats []struct {
		Type string `bson:"type"`
	}
	if err := cursor.All(ctx, &stats); err != nil {
		return false, fmt.Errorf("reading the cluster storage type: %w", err)
	}

	for _, stat := range stats {
		if stat.Type == disaggregatedStorageType {
			return true, nil
		}
	}

	return false, nil
}
