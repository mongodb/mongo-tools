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

func TestNumberIntValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "NumberInt(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(NumberInt)
		require.True(t, ok)
		assert.Equal(t, NumberInt(123), jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "NumberInt(123)", "NumberInt(456)", "NumberInt(789)"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(NumberInt)
		require.True(t, ok)
		assert.Equal(t, NumberInt(123), jsonValue1)

		jsonValue2, ok := jsonMap[key2].(NumberInt)
		require.True(t, ok)
		assert.Equal(t, NumberInt(456), jsonValue2)

		jsonValue3, ok := jsonMap[key3].(NumberInt)
		require.True(t, ok)
		assert.Equal(t, NumberInt(789), jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "NumberInt(42)"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(NumberInt)
			require.True(t, ok)
			assert.Equal(t, NumberInt(42), jsonValue)
		}
	})

	t.Run("string arg", func(t *testing.T) {
		key := "key"
		value := `NumberInt("123")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		jsonValue, err := UnmarshalBsonD([]byte(data))

		assert.Equal(t, NumberInt(123), jsonValue[0].Value)
		require.NoError(t, err)
	})

	t.Run("hexadecimal arg", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "NumberInt(0x5f)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(NumberInt)
		require.True(t, ok)
		assert.Equal(t, NumberInt(0x5f), jsonValue)
	})
}

func TestNumberLongValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "NumberLong(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(NumberLong)
		require.True(t, ok)
		assert.Equal(t, NumberLong(123), jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "NumberLong(123)", "NumberLong(456)", "NumberLong(789)"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(NumberLong)
		require.True(t, ok)
		assert.Equal(t, NumberLong(123), jsonValue1)

		jsonValue2, ok := jsonMap[key2].(NumberLong)
		require.True(t, ok)
		assert.Equal(t, NumberLong(456), jsonValue2)

		jsonValue3, ok := jsonMap[key3].(NumberLong)
		require.True(t, ok)
		assert.Equal(t, NumberLong(789), jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "NumberLong(42)"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(NumberLong)
			require.True(t, ok)
			assert.Equal(t, NumberLong(42), jsonValue)
		}
	})

	t.Run("string arg", func(t *testing.T) {
		key := "key"
		value := `NumberLong("123")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		jsonValue, err := UnmarshalBsonD([]byte(data))

		assert.Equal(t, NumberLong(123), jsonValue[0].Value)
		require.NoError(t, err)
	})

	t.Run("hexadecimal arg", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "NumberLong(0x5f)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(NumberLong)
		require.True(t, ok)
		assert.Equal(t, NumberLong(0x5f), jsonValue)
	})
}
