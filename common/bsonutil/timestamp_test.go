package bsonutil

import (
	"github.com/mongodb/mongo-tools/common/json"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"testing"
)

func TestTimestampValue(t *testing.T) {

	Convey("When converting JSON with Timestamp values", t, func() {
		testTS := bson.MongoTimestamp(123456<<32 | 55) //TODO submit a pull req to mgo

		Convey("works for Timestamp literal", func() {

			jsonMap := map[string]interface{}{
				"ts": json.Timestamp{123456, 55},
			}

			err := ConvertJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			So(jsonMap["ts"], ShouldEqual, testTS)
		})

		Convey(`works for Timestamp document`, func() {
			Convey(`{"ts":{"$timestamp":{"t":123456, "i":55}}}`, func() {
				jsonMap := map[string]interface{}{
					"ts": map[string]interface{}{
						"$timestamp": map[string]interface{}{
							"t": 123456.0,
							"i": 55.0,
						},
					},
				}

				bsonMap, err := ConvertJSONValueToBSON(jsonMap)
				So(err, ShouldBeNil)
				So(bsonMap.(map[string]interface{})["ts"], ShouldEqual, testTS)
			})
		})
	})
}
