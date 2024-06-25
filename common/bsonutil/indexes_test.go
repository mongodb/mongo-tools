// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		if result := IsIndexKeysEqual(test.IndexKeys1, test.IndexKeys2); result != test.Expected {
			t.Fatalf(
				"Wrong output from IsIndexKeysEqual as expected, test: %v, actual: %v",
				test,
				result,
			)
		}
	}
}

func TestConvertLegacyIndexKeys(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("Converting legacy Indexes", t, func() {
		index1Key := bson.D{{"foo", int32(0)}, {"int32field", int32(2)},
			{
				"int64field",
				int64(-3),
			}, {"float64field", float64(-1)}, {"float64field", float64(-1.1)}}
		ConvertLegacyIndexKeys(index1Key, "test")
		So(
			index1Key,
			ShouldResemble,
			bson.D{{"foo", int32(1)}, {"int32field", int32(2)}, {"int64field", int64(-3)},
				{"float64field", float64(-1)}, {"float64field", float64(-1.1)}},
		)

		decimalNOne, _ := primitive.ParseDecimal128("-1")
		decimalZero, _ := primitive.ParseDecimal128("0")
		decimalOne, _ := primitive.ParseDecimal128("1")
		decimalZero1, _ := primitive.ParseDecimal128("0.00")
		index2Key := bson.D{
			{"key1", decimalNOne},
			{"key2", decimalZero},
			{"key3", decimalOne},
			{"key4", decimalZero1},
		}
		ConvertLegacyIndexKeys(index2Key, "test")
		So(
			index2Key,
			ShouldResemble,
			bson.D{
				{"key1", decimalNOne},
				{"key2", int32(1)},
				{"key3", decimalOne},
				{"key4", int32(1)},
			},
		)

		index3Key := bson.D{{"key1", ""}, {"key2", "2dsphere"}}
		ConvertLegacyIndexKeys(index3Key, "test")
		So(index3Key, ShouldResemble, bson.D{{"key1", int32(1)}, {"key2", "2dsphere"}})

		index4Key := bson.D{{"key1", bson.E{"invalid", 1}}, {"key2", primitive.Binary{}}}
		ConvertLegacyIndexKeys(index4Key, "test")
		So(index4Key, ShouldResemble, bson.D{{"key1", int32(1)}, {"key2", int32(1)}})
	})
}

func TestConvertLegacyIndexOptionsFromOp(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	convertedIndex := bson.D{{"v", int32(1)}, {"key", bson.D{{"a", int32(1)}}},
		{"name", "a_1"}, {"unique", true}}

	Convey("Converting legacy index options", t, func() {
		indexoptionsNoInvalidOption := bson.D{{"v", int32(1)}, {"key", bson.D{{"a", int32(1)}}},
			{"name", "a_1"}, {"unique", true}}
		ConvertLegacyIndexOptionsFromOp(&indexoptionsNoInvalidOption)
		So(indexoptionsNoInvalidOption, ShouldResemble, convertedIndex)

		indexoptionsSingleInvalidOption := bson.D{{"v", int32(1)}, {"key", bson.D{{"a", int32(1)}}},
			{"name", "a_1"}, {"unique", true}, {"invalid_option", int32(1)}}
		ConvertLegacyIndexOptionsFromOp(&indexoptionsSingleInvalidOption)
		So(indexoptionsSingleInvalidOption, ShouldResemble, convertedIndex)

		indexoptionsMultipleInvalidOptions := bson.D{
			{"v", int32(1)},
			{"key", bson.D{{"a", int32(1)}}},
			{
				"name",
				"a_1",
			},
			{"unique", true},
			{"invalid_option_1", true},
			{"invalid_option_2", int32(1)},
		}
		ConvertLegacyIndexOptionsFromOp(&indexoptionsMultipleInvalidOptions)
		So(indexoptionsMultipleInvalidOptions, ShouldResemble, convertedIndex)
	})
}
