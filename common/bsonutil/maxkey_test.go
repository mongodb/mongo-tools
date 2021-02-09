// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"

	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMaxKeyValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When converting JSON with MaxKey values", t, func() {

		Convey("works for MaxKey literal", func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: json.MaxKey{},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldResemble, primitive.MaxKey{})
		})

		Convey(`works for MaxKey document ('{ "$maxKey": 1 }')`, func() {
			key := "maxKey"
			jsonMap := map[string]interface{}{
				key: map[string]interface{}{
					"$maxKey": 1,
				},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap[key], ShouldResemble, primitive.MaxKey{})
		})
	})
}
