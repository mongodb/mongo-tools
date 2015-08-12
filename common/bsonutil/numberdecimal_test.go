package bsonutil

import (
	"github.com/mongodb/mongo-tools/common/json"
	. "github.com/smartystreets/goconvey/convey"
	"testing"

	"gopkg.in/mgo.v2/decimal"
)

func TestNumberDecimalValue(t *testing.T) {

	Convey("When converting JSON with NumberDecimal values", t, func() {

		Convey("works for NumberDecimal constructor", func() {
			dcmlIn, _ := decimal.Parse("0.1")
			key := "key"
			jsonMap := map[string]interface{}{
				key: json.NumberDecimal(dcmlIn),
			}

			err := ConvertJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			dcmlOut, _ := decimal.Parse("0.1")
			So(jsonMap[key], ShouldResemble, dcmlOut)
		})

		Convey(`works for NumberDecimal document ('{ "$numberDecimal": "0.1" }')`, func() {
			key := "key"
			jsonMap := map[string]interface{}{
				key: map[string]interface{}{
					"$numberDecimal": "0.1",
				},
			}

			err := ConvertJSONDocumentToBSON(jsonMap)
			So(err, ShouldBeNil)
			dcmlOut, _ := decimal.Parse("0.1")
			So(jsonMap[key], ShouldResemble, dcmlOut)
		})
	})
}
