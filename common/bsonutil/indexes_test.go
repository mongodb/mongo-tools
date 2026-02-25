// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
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

	index1Key := bson.D{
		{"foo", int32(0)},
		{"int32field", int32(2)},
		{"int64field", int64(-3)},
		{"float64field", float64(0)},
		{"float64field", float64(-1)},
		{"float64field", float64(-1.1)},
		{"float64field", float64(1e-9)},
		{"float64field", float64(-1e-9)},
		{"float64field", float64(1e-10)},
		{"float64field", float64(-1e-10)},
	}

	ConvertLegacyIndexKeys(index1Key, "test")

	assert.Equal(
		t,
		bson.D{
			{"foo", int32(1)},
			{"int32field", int32(2)},
			{"int64field", int64(-3)},
			{"float64field", int32(1)},
			{"float64field", float64(-1)},
			{"float64field", float64(-1.1)},
			{"float64field", float64(1e-9)},
			{"float64field", float64(-1e-9)},
			{"float64field", int32(1)},
			{"float64field", int32(-1)},
		},
		index1Key,
	)

	decimalNOne, _ := bson.ParseDecimal128("-1")
	decimalZero, _ := bson.ParseDecimal128("0")
	decimalOne, _ := bson.ParseDecimal128("1")
	decimalZero1, _ := bson.ParseDecimal128("0.00")
	index2Key := bson.D{
		{"key1", decimalNOne},
		{"key2", decimalZero},
		{"key3", decimalOne},
		{"key4", decimalZero1},
	}

	ConvertLegacyIndexKeys(index2Key, "test")

	assert.Equal(
		t,
		bson.D{
			{"key1", decimalNOne},
			{"key2", int32(1)},
			{"key3", decimalOne},
			{"key4", int32(1)},
		},
		index2Key,
	)

	index3Key := bson.D{{"key1", ""}, {"key2", "2dsphere"}}
	ConvertLegacyIndexKeys(index3Key, "test")
	assert.Equal(
		t,
		bson.D{{"key1", int32(1)}, {"key2", "2dsphere"}},
		index3Key,
	)

	index4Key := bson.D{{"key1", bson.E{"invalid", 1}}, {"key2", bson.Binary{}}}
	ConvertLegacyIndexKeys(index4Key, "test")
	assert.Equal(
		t,
		bson.D{{"key1", int32(1)}, {"key2", int32(1)}},
		index4Key,
	)
}
