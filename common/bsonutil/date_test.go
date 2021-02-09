// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
)

const rfc3339Milli = "2006-01-02T15:04:05.999Z07:00"

func TestDateValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When converting JSON with Date values", t, func() {
		Convey("works for Date document", func() {

			dates := []string{
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05.000-07:00",
				"2006-01-02T15:04:05Z",
				"2006-01-02T15:04:05-07:00",
			}

			for _, dateString := range dates {
				example := fmt.Sprintf(`{ "$date": "%v" }`, dateString)
				Convey(fmt.Sprintf("of string ('%v')", example), func() {
					key := "key"
					jsonMap := map[string]interface{}{
						key: map[string]interface{}{
							"$date": dateString,
						},
					}

					err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
					So(err, ShouldBeNil)

					// dateString is a valid time format string
					date, err := time.Parse(rfc3339Milli, dateString)
					So(err, ShouldBeNil)

					jsonValue, ok := jsonMap[key].(time.Time)
					So(ok, ShouldBeTrue)
					So(jsonValue, ShouldEqual, date)
				})
			}

			date := time.Unix(0, int64(time.Duration(1136214245000)*time.Millisecond))

			Convey(`of $numberLong ('{ "$date": { "$numberLong": "1136214245000" } }')`, func() {
				key := "key"
				jsonMap := map[string]interface{}{
					key: map[string]interface{}{
						"$date": map[string]interface{}{
							"$numberLong": "1136214245000",
						},
					},
				}

				err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
				So(err, ShouldBeNil)

				jsonValue, ok := jsonMap[key].(time.Time)
				So(ok, ShouldBeTrue)
				So(jsonValue, ShouldEqual, date)
			})
		})
	})
}
