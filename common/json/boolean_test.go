// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package json

import (
	"fmt"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBooleanValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)
		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)
	})

	t.Run("no args", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean()"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)
		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("struct of specific type", func(t *testing.T) {
		type TestStruct struct {
			A bool
			//nolint:unused
			b int
		}
		var jsonStruct TestStruct

		key := "A"
		value := "Boolean(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonStruct)
		require.NoError(t, err)
		assert.True(t, jsonStruct.A)

		key = "A"
		value = "Boolean(0)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonStruct)
		require.NoError(t, err)
		assert.False(t, jsonStruct.A)
	})

	t.Run("explicit bool", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(true)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean(false)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("numbers", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(1)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean(0)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)

		value = "Boolean(0.0)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)

		value = "Boolean(2.0)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean(-15.4)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)
	})

	t.Run("strings", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean('hello')"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean('')"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("undefined", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(undefined)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("null", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(null)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("too many args", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(true, false)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		key = "key"
		value = "Boolean(false, true)"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "Boolean(123)", "Boolean(0)", "Boolean(true)"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue3)
	})

	t.Run("other types", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(new Date (0))"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean(ObjectId('56609335028bd7dc5c36cb9f'))"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean([])"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)
	})

	t.Run("nested booleans", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(Boolean(5))"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		value = "Boolean(Boolean(Boolean(0)))"
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value1 := "Boolean(42)"
		value2 := "Boolean(0)"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value1, value2, value1)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		jsonValue, ok := jsonArray[0].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		jsonValue, ok = jsonArray[1].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)

		jsonValue, ok = jsonArray[2].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)
	})

	t.Run("specify true in hexadecimal", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(0x5f)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)
	})

	t.Run("specify false in hexadecimal", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Boolean(0x0)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})
}
