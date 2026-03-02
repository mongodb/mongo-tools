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

func TestHexadecimalNumber(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	value := "0x123"
	intValue := 0x123

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any
		key := "key"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)
		jsonValue, ok := jsonMap[key].(int32)
		require.True(t, ok)
		assert.EqualValues(t, intValue, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "0x100", "0x101", "0x102"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(int32)
		require.True(t, ok)
		assert.EqualValues(t, 0x100, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(int32)
		require.True(t, ok)
		assert.EqualValues(t, 0x101, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(int32)
		require.True(t, ok)
		assert.EqualValues(t, 0x102, jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(int32)
			require.True(t, ok)
			assert.EqualValues(t, intValue, jsonValue)
		}
	})

	t.Run("with sign", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		data := fmt.Sprintf(`{"%v":+%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(int32)
		require.True(t, ok)
		assert.EqualValues(t, intValue, jsonValue)

		data = fmt.Sprintf(`{"%v":-%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(int32)
		require.True(t, ok)
		assert.EqualValues(t, -intValue, jsonValue)
	})

	t.Run("with '0x' or '0X' prefix", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "123"
		data := fmt.Sprintf(`{"%v":0x%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(int32)
		require.True(t, ok)
		assert.EqualValues(t, intValue, jsonValue)

		data = fmt.Sprintf(`{"%v":0X%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(int32)
		require.True(t, ok)
		assert.EqualValues(t, intValue, jsonValue)
	})
}
