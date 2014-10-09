package json

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestMaxKeyValue(t *testing.T) {

	Convey("When unmarshalling JSON with MaxKey values", t, func() {

		Convey("works for a single key", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "MaxKey"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue, ok := jsonMap[key].(MaxKey)
			So(ok, ShouldBeTrue)
			So(jsonValue, ShouldResemble, MaxKey{})
		})

		Convey("works for multiple keys", func() {
			var jsonMap map[string]interface{}

			key1, key2, key3 := "key1", "key2", "key3"
			value := "MaxKey"
			data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
				key1, value, key2, value, key3, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonValue1, ok := jsonMap[key1].(MaxKey)
			So(ok, ShouldBeTrue)
			So(jsonValue1, ShouldResemble, MaxKey{})

			jsonValue2, ok := jsonMap[key2].(MaxKey)
			So(ok, ShouldBeTrue)
			So(jsonValue2, ShouldResemble, MaxKey{})

			jsonValue3, ok := jsonMap[key3].(MaxKey)
			So(ok, ShouldBeTrue)
			So(jsonValue3, ShouldResemble, MaxKey{})
		})

		Convey("works in an array", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "MaxKey"
			data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
				key, value, value, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldBeNil)

			jsonArray, ok := jsonMap[key].([]interface{})
			So(ok, ShouldBeTrue)

			for _, _jsonValue := range jsonArray {
				jsonValue, ok := _jsonValue.(MaxKey)
				So(ok, ShouldBeTrue)
				So(jsonValue, ShouldResemble, MaxKey{})
			}
		})

		Convey("cannot have a sign ('+' or '-')", func() {
			var jsonMap map[string]interface{}

			key := "key"
			value := "MaxKey"
			data := fmt.Sprintf(`{"%v":+%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldNotBeNil)

			data = fmt.Sprintf(`{"%v":-%v}`, key, value)

			err = Unmarshal([]byte(data), &jsonMap)
			So(err, ShouldNotBeNil)
		})
	})
}
