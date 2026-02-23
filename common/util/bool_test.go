// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"math"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestJSTruthyValues(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	trueCases := []any{
		// some edge cases
		true,
		map[string]any(nil),
		map[string]any{"a": 1},
		[]byte(nil),
		[]byte{21, 12},
		"",
		math.NaN(),

		// normal cases
		[]int{1, 2, 3},
		"true",
		"false",
		25,
		25.1,
		struct{ A int }{A: 12},
	}

	falseCases := []any{
		false,
		0,
		float64(0),
		nil,
		bson.Undefined{},
	}

	t.Run("truthy cases", func(t *testing.T) {
		for _, val := range trueCases {
			assert.True(t, IsTruthy(val), "%v -> true", val)
		}
	})

	t.Run("falsy cases", func(t *testing.T) {
		for _, val := range falseCases {
			assert.False(t, IsTruthy(val), "%v -> false", val)
		}
	})
}
