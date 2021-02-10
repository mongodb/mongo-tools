// Copyright (C) MongoDB, Inc. 2019-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestTimestampComparisons(t *testing.T) {
	t.Run("TestTimestampGreaterThan", func(t *testing.T) {
		reference := primitive.Timestamp{T: 5, I: 5}

		cases := []struct {
			name     string
			lhs, rhs primitive.Timestamp
			expected bool
		}{
			{"different T", primitive.Timestamp{T: 1000, I: 0}, reference, true},
			{"equal T", primitive.Timestamp{T: 5, I: 1}, reference, false},
			{"equal T and I", reference, reference, false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if res := TimestampGreaterThan(tc.lhs, tc.rhs); res != tc.expected {
					t.Fatalf("comparison mismatch; expected %v, got %v", tc.expected, res)
				}
			})
		}
	})

	t.Run("TestTimestampGreaterThan", func(t *testing.T) {
		reference := primitive.Timestamp{T: 1000, I: 5}

		cases := []struct {
			name     string
			lhs, rhs primitive.Timestamp
			expected bool
		}{
			{"different T", primitive.Timestamp{T: 5, I: 0}, reference, true},
			{"equal T", primitive.Timestamp{T: 1000, I: 10}, reference, false},
			{"equal T and I", reference, reference, false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if res := TimestampLessThan(tc.lhs, tc.rhs); res != tc.expected {
					t.Fatalf("comparison mismatch; expected %v, got %v", tc.expected, res)
				}
			})
		}
	})
}
