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

func TestBinDataValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `BinData(1, "xyz")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{1, "xyz"}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := `BinData(1, "abc")`,
			`BinData(2, "def")`, `BinData(3, "ghi")`
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{1, "abc"}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{2, "def"}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{3, "ghi"}, jsonValue3)
	})

	t.Run("array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `BinData(42, "10")`
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(BinData)
			require.True(t, ok)
			assert.Equal(t, BinData{42, "10"}, jsonValue)
		}
	})

	t.Run("specify type argument", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `BinData(0x5f, "xyz")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{0x5f, "xyz"}, jsonValue)
	})
}
