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

func TestObjectIdValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	oid, _ := primitive.ObjectIDFromHex("0123456789abcdef01234567")

	Convey("When converting JSON with ObjectId values", t, func() {

		Convey("works for ObjectId constructor", func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: json.ObjectId("0123456789abcdef01234567"),
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldEqual, oid)
		})

		Convey(`works for ObjectId document ('{ "$oid": "0123456789abcdef01234567" }')`, func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: map[string]interface{}{
					"$oid": "0123456789abcdef01234567",
				},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldEqual, oid)
		})
	})
}
