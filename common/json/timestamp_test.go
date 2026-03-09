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

func TestTimestampValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Timestamp(123, 321)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(Timestamp)
		require.True(t, ok)
		assert.Equal(t, Timestamp{123, 321}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "Timestamp(123, 321)",
			"Timestamp(456, 654)", "Timestamp(789, 987)"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(Timestamp)
		require.True(t, ok)
		assert.Equal(t, Timestamp{123, 321}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(Timestamp)
		require.True(t, ok)
		assert.Equal(t, Timestamp{456, 654}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(Timestamp)
		require.True(t, ok)
		assert.Equal(t, Timestamp{789, 987}, jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "Timestamp(42, 10)"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(Timestamp)
			require.True(t, ok)
			assert.Equal(t, Timestamp{42, 10}, jsonValue)
		}
	})

	t.Run("string argument", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `Timestamp("123", "321")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
