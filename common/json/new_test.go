package json

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"

	"gopkg.in/mgo.v2/decimal"
)

func TestNewKeyword(t *testing.T) {

	Convey("When unmarshalling JSON using the new keyword", t, func() {

		Convey("can be used with BinData constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `new BinData(1, "xyz")`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(BinData)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldResemble, BinData{1, "xyz"})
		})

		Convey("can be used with Date constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "new Date(123)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(Date)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, Date(123))
		})

		Convey("can be used with DBRef constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `new BinData(1, "xyz")`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(BinData)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldResemble, BinData{1, "xyz"})
		})

		Convey("can be used with NumberInt constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "new NumberInt(123)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(NumberInt)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, NumberInt(123))
		})

		Convey("can be used with NumberLong constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "new NumberLong(123)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(NumberLong)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, NumberLong(123))
		})

		Convey("can be used with NumberDecimal constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `new NumberDecimal(123)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(NumberDecimal)
			So(ok, ShouldBeTrue)
			dcml, _ := decimal.Parse("123")
			So(jsonValue, ShouldResemble, NumberDecimal(dcml))
		})

		Convey("can be used with ObjectId constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `new ObjectId("123")`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(ObjectId)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldEqual, ObjectId("123"))
		})

		Convey("can be used with RegExp constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := `new RegExp("foo", "i")`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(RegExp)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldResemble, RegExp{"foo", "i"})
		})

		Convey("can be used with Timestamp constructor", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "new Timestamp(123, 321)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(Timestamp)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldResemble, Timestamp{123, 321})
		})

		Convey("cannot be used with literals", func() {
			var jsonMap map[string]interface{}

			key := "key"
			literals := []string{"null", "true", "false", "undefined",
				"NaN", "Infinity", "MinKey", "MaxKey"}

			for _, value := range literals {
				data := fmt.Sprintf(`{"%v":new %v}`, key, value)
				Convey(value, func() {
					err := Unmarshal([]byte(data), &jsonMap)
					So(err, ShouldNotBeNil)
				})
			}
		})

		Convey("must be followed by a space", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "newDate(123)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldNotBeNil)
		})

		Convey("cannot be chained togther (`new new ...`)", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "new new Date(123)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldNotBeNil)
		})
	})
}
