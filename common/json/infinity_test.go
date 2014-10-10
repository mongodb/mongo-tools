package json

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"math"
	"testing"
)

func TestInfinityValue(t *testing.T) {

	Convey("When unmarshalling JSON with Infinity values", t, func() {

		Convey("works for a single key", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "Infinity"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(math.IsInf(jsonValue, 1), ShouldBeTrue)
		})

		Convey("works for multiple keys", func() {
			var jsonMap map[string]interface{}

			key1, key2, key3 := "key1", "key2", "key3"
			value := "Infinity"
			data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
				key1, value, key2, value, key3, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue1, ok := jsonMap[key1].(float64)
			So(ok, ShouldBeTrue)
			So(math.IsInf(jsonValue1, 1), ShouldBeTrue)

			jsonValue2, ok := jsonMap[key2].(float64)
			So(ok, ShouldBeTrue)
			So(math.IsInf(jsonValue2, 1), ShouldBeTrue)

			jsonValue3, ok := jsonMap[key3].(float64)
			So(ok, ShouldBeTrue)
			So(math.IsInf(jsonValue3, 1), ShouldBeTrue)
		})

		Convey("works in an array", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "Infinity"
			data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
				key, value, value, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonArray, ok := jsonMap[key].([]interface{})
			So(ok, ShouldBeTrue)

			for _, _jsonValue := range jsonArray {
				jsonValue, ok := _jsonValue.(float64)
				So(ok, ShouldBeTrue)
				So(math.IsInf(jsonValue, 1), ShouldBeTrue)
			}
		})

		Convey("can have a sign ('+' or '-')", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "Infinity"
			data := fmt.Sprintf(`{"%v":+%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(math.IsInf(jsonValue, 1), ShouldBeTrue)

			data = fmt.Sprintf(`{"%v":-%v}`, key, value)

			err = Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok = jsonMap[key].(float64)
			So(ok, ShouldBeTrue)
			So(math.IsInf(jsonValue, -1), ShouldBeTrue)
		})
	})
}
