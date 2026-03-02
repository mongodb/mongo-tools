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

func TestFractionalNumber(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	epsilon := 0.000001

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := ".123"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(float64)
		require.True(t, ok)
		assert.InDelta(t, 0.123, jsonValue, epsilon)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := ".123", ".456", ".789"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(float64)
		require.True(t, ok)
		assert.InDelta(t, 0.123, jsonValue1, epsilon)

		jsonValue2, ok := jsonMap[key2].(float64)
		require.True(t, ok)
		assert.InDelta(t, 0.456, jsonValue2, epsilon)

		jsonValue3, ok := jsonMap[key3].(float64)
		require.True(t, ok)
		assert.InDelta(t, 0.789, jsonValue3, epsilon)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := ".42"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(float64)
			require.True(t, ok)
			assert.InDelta(t, 0.42, jsonValue, epsilon)
		}
	})

	t.Run("with sign", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := ".106"
		data := fmt.Sprintf(`{"%v":+%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(float64)
		require.True(t, ok)
		assert.InDelta(t, 0.106, jsonValue, epsilon)

		data = fmt.Sprintf(`{"%v":-%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(float64)
		require.True(t, ok)
		assert.InDelta(t, -0.106, jsonValue, epsilon)
	})
}
