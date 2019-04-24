// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package json

import (
	"fmt"
	"github.com/mongodb/mongo-tools/legacy/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestObjectIdValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When unmarshalling JSON with ObjectId values", t, func() {

		Convey("works for a single key", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `ObjectId("123")`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(ObjectId)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, ObjectId("123"))
		})

		Convey("works for multiple keys", func() {
			var jsonMap map[string]interface{}

			key1, key2, key3 := "key1", "key2", "key3"
			value1, value2, value3 := `ObjectId("123")`, `ObjectId("456")`, `ObjectId("789")`
			data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
				key1, value1, key2, value2, key3, value3)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue1, ok := jsonMap[key1].(ObjectId)
			So(ok, ShouldBeTrue)
			So(jsonValue1, ShouldEqual, ObjectId("123"))

			jsonValue2, ok := jsonMap[key2].(ObjectId)
			So(ok, ShouldBeTrue)
			So(jsonValue2, ShouldEqual, ObjectId("456"))

			jsonValue3, ok := jsonMap[key3].(ObjectId)
			So(ok, ShouldBeTrue)
			So(jsonValue3, ShouldEqual, ObjectId("789"))
		})

		Convey("works in an array", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `ObjectId("000")`
			data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
				key, value, value, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonArray, ok := jsonMap[key].([]interface{})
			So(ok, ShouldBeTrue)

			for _, _jsonValue := range jsonArray {
				jsonValue, ok := _jsonValue.(ObjectId)
				So(ok, ShouldBeTrue)
				So(jsonValue, ShouldEqual, ObjectId("000"))
			}
		})

		Convey("cannot use number as argument", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `ObjectId(123)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldNotBeNil)
		})
	})
}
