// Copyright (C) MongoDB, Inc. 2019-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import "go.mongodb.org/mongo-driver/bson/primitive"

// CompareTimestamps returns -1 if lhs comes before rhs, 0 if they're equal, and 1 if rhs comes after lhs.
func TimestampGreaterThan(lhs, rhs primitive.Timestamp) bool {
	return lhs.T > rhs.T || lhs.T == rhs.T && lhs.I > rhs.I
}
