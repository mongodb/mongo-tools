// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestRegExpValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When converting JSON with RegExp values", t, func() {

		Convey("works for RegExp constructor", func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: json.RegExp{"foo", "i"},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldResemble, primitive.Regex{"foo", "i"})
		})

		Convey(`works for RegExp document ('{ "$regex": "foo", "$options": "i" }')`, func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: map[string]interface{}{
					"$regex":   "foo",
					"$options": "i",
				},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldResemble, primitive.Regex{"foo", "i"})
		})

		Convey(`can use multiple options ('{ "$regex": "bar", "$options": "gims" }')`, func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: map[string]interface{}{
					"$regex":   "bar",
					"$options": "gims",
				},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldResemble, primitive.Regex{"bar", "gims"})
		})

		Convey(`fails for an invalid option ('{ "$regex": "baz", "$options": "y" }')`, func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: map[string]interface{}{
					"$regex":   "baz",
					"$options": "y",
				},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldNotBeNil)
		})
	})
}
