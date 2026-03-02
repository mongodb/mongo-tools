// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package json

import (
	"fmt"
	"math"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBRefValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `DBRef("ref", "123")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(DBRef)
		require.True(t, ok)
		assert.Equal(t, DBRef{"ref", "123", ""}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := `DBRef("ref1", "123")`,
			`DBRef("ref2", "456")`, `DBRef("ref3", "789")`
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(DBRef)
		require.True(t, ok)
		assert.Equal(t, DBRef{"ref1", "123", ""}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(DBRef)
		require.True(t, ok)
		assert.Equal(t, DBRef{"ref2", "456", ""}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(DBRef)
		require.True(t, ok)
		assert.Equal(t, DBRef{"ref3", "789", ""}, jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `DBRef("ref", "42")`
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", "42", ""}, jsonValue)
		}
	})

	t.Run("alternative capitalization", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `Dbref("ref", "123")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(DBRef)
		require.True(t, ok)
		assert.Equal(t, DBRef{"ref", "123", ""}, jsonValue)
	})

	t.Run("different types for id parameter", func(t *testing.T) {

		t.Run("null literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", null)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", nil, ""}, jsonValue)
		})

		t.Run("true literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", true)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", true, ""}, jsonValue)
		})

		t.Run("false literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", false)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", false, ""}, jsonValue)
		})

		t.Run("undefined literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", undefined)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", Undefined{}, ""}, jsonValue)
		})

		t.Run("NaN literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", NaN)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, "ref", jsonValue.Collection)

			id, ok := jsonValue.Id.(float64)
			require.True(t, ok)
			assert.True(t, math.IsNaN(id))

		})

		t.Run("Infinity literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", Infinity)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, "ref", jsonValue.Collection)

			id, ok := jsonValue.Id.(float64)
			require.True(t, ok)
			assert.True(t, math.IsInf(id, 1))

		})

		t.Run("MinKey literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", MinKey)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", MinKey{}, ""}, jsonValue)
		})

		t.Run("MaxKey literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", MaxKey)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", MaxKey{}, ""}, jsonValue)
		})

		t.Run("ObjectId object", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", ObjectId("123"))`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", ObjectId("123"), ""}, jsonValue)
		})

		t.Run("NumberInt object", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", NumberInt(123))`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", NumberInt(123), ""}, jsonValue)
		})

		t.Run("NumberLong object", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", NumberLong(123))`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", NumberLong(123), ""}, jsonValue)
		})

		t.Run("RegExp object", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", RegExp("xyz", "i"))`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", RegExp{"xyz", "i"}, ""}, jsonValue)
		})

		t.Run("regular expression literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", /xyz/i)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", RegExp{"xyz", "i"}, ""}, jsonValue)
		})

		t.Run("Timestamp object", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", Timestamp(123, 321))`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", Timestamp{123, 321}, ""}, jsonValue)
		})

		t.Run("string literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", "xyz")`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, DBRef{"ref", "xyz", ""}, jsonValue)
		})

		t.Run("numeric literal", func(t *testing.T) {
			var jsonMap map[string]any

			key := "key"
			value := `DBRef("ref", 123)`
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(DBRef)
			require.True(t, ok)
			assert.Equal(t, "ref", jsonValue.Collection)

			id, ok := jsonValue.Id.(int32)
			require.True(t, ok)
			assert.InDelta(t, 123, id, 0.000000001)
		})
	})
}
