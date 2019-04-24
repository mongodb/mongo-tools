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

func TestNumberFloatValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When unmarshaling JSON with float values", t, func() {

		Convey("converts to a JSON NumberFloat value", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "5.5"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, NumberFloat(5.5))

		})
	})

	Convey("When unmarshaling and marshaling NumberFloat values", t, func() {
		key := "key"

		Convey("maintains decimal point with trailing zero", func() {
			var jsonMap map[string]interface{}

			value := "5.0"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, NumberFloat(5.0))

			numFloat := NumberFloat(jsonValue)
			byteValue, err := numFloat.MarshalJSON()
			So(err, ShouldBeNil)
			So(string(byteValue), ShouldEqual, "5.0")

		})

		Convey("maintains precision with large decimals", func() {
			var jsonMap map[string]interface{}

			value := "5.52342123"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, NumberFloat(5.52342123))

			numFloat := NumberFloat(jsonValue)
			byteValue, err := numFloat.MarshalJSON()
			So(err, ShouldBeNil)
			So(string(byteValue), ShouldEqual, "5.52342123")

		})

		Convey("maintains exponent values", func() {
			var jsonMap map[string]interface{}

			value := "5e+32"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, NumberFloat(5e32))

			numFloat := NumberFloat(jsonValue)
			byteValue, err := numFloat.MarshalJSON()
			So(err, ShouldBeNil)
			So(string(byteValue), ShouldEqual, "5e+32")

		})
	})
}
