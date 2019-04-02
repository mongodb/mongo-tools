// Copyright (C) MongoDB, Inc. 2019-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import "go.mongodb.org/mongo-driver/bson/primitive"

// CompareTimestamps returns -1 if lhs comes before rhs, 0 if they're equal, and 1 if rhs comes after lhs.
func CompareTimestamps(lhs, rhs primitive.Timestamp) int {
	if lhs.T == rhs.T {
		if lhs.I > rhs.I {
			return 1
		} else if lhs.I < rhs.I {
			return -1
		} else {
			return 0
		}
	} else if lhs.T > rhs.T {
		return 1
	} else {
		return -1
	}
}
