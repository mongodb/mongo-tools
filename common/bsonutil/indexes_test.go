// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"fmt"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestIsIndexKeysEqual(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tests := []struct {
		IndexKeys1 bson.D
		IndexKeys2 bson.D
		Expected   bool
	}{
		{bson.D{{"a", int32(1)}, {"b", int32(1)}},
			bson.D{{"a", int32(1)}, {"b", int64(1)}},
			true},
		{bson.D{{"a", int32(1)}, {"b", int32(1)}},
			bson.D{{"a", float64(1)}, {"b", int64(1)}},
			true},
		{bson.D{{"a", -1.0}, {"b", 1.0}},
			bson.D{{"a", int32(-1)}, {"b", int32(1)}},
			true},
		{bson.D{{"a", -2.0}},
			bson.D{{"a", int32(-1)}},
			false},
		{bson.D{{"b", int32(1)}},
			bson.D{{"a", int32(1)}},
			false},
		{bson.D{{"a", int32(1)}, {"b", int32(1)}},
			bson.D{{"b", int32(1)}, {"a", int32(1)}},
			false},
		{bson.D{{"a", int32(1)}, {"b", int32(1)}},
			bson.D{{"a", int32(1)}},
			false},
	}

	for _, test := range tests {
		assert.Equal(
			t,
			test.Expected,
			IsIndexKeysEqual(test.IndexKeys1, test.IndexKeys2),
			"for test %v",
			test,
		)
	}
}

func TestConvertLegacyIndexKeys(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	type testCase struct {
		input           any
		expect          any
		expectConverted bool
	}

	decimalNegOne, _ := bson.ParseDecimal128("-1")
	decimalZero, _ := bson.ParseDecimal128("0")
	decimalOne, _ := bson.ParseDecimal128("1")
	decimalZero1, _ := bson.ParseDecimal128("0.00")

	tests := []testCase{
		{int32(0), int32(1), true},
		{int32(2), int32(2), false},
		{int64(-3), int64(-3), false},
		{float64(0), int32(1), true},
		{float64(-1), float64(-1), false},
		{float64(-1.1), float64(-1.1), false},
		{float64(1e-9), float64(1e-9), false},
		{float64(-1e-9), float64(-1e-9), false},
		{float64(1e-10), int32(1), true},
		{float64(-1e-10), int32(-1), true},
		{decimalNegOne, decimalNegOne, false},
		{decimalZero, int32(1), true},
		{decimalOne, decimalOne, false},
		{decimalZero1, int32(1), true},
		{"", int32(1), true},
		{"2dsphere", "2dsphere", false},
		{bson.Binary{}, int32(1), true},
	}

	for _, test := range tests {
		t.Run(
			fmt.Sprintf("%T(%v)", test.input, test.input),
			func(t *testing.T) {
				got, converted := ConvertLegacyIndexKeyValue(test.input)
				assert.Equal(t, test.expect, got, "got expected value back")
				if test.expectConverted {
					assert.True(t, converted, "value was converted")
				} else {
					assert.False(t, converted, "value was not converted")
				}
			},
		)
	}
}
