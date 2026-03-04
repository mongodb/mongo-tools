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

func TestMinKeyValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	key := "key"

	t.Run("MinKey, no parens", func(t *testing.T) {
		value := "MinKey"

		t.Run("single key", func(t *testing.T) {
			var jsonMap map[string]any

			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue)
		})

		t.Run("multiple keys", func(t *testing.T) {
			var jsonMap map[string]any

			key1, key2, key3 := "key1", "key2", "key3"
			data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
				key1, value, key2, value, key3, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue1, ok := jsonMap[key1].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue1)

			jsonValue2, ok := jsonMap[key2].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue2)

			jsonValue3, ok := jsonMap[key3].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue3)
		})

		t.Run("in array", func(t *testing.T) {
			var jsonMap map[string]any

			data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
				key, value, value, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonArray, ok := jsonMap[key].([]any)
			require.True(t, ok)

			for _, _jsonValue := range jsonArray {
				jsonValue, ok := _jsonValue.(MinKey)
				require.True(t, ok)
				assert.Equal(t, MinKey{}, jsonValue)
			}
		})

		t.Run("no signs", func(t *testing.T) {
			var jsonMap map[string]any

			data := fmt.Sprintf(`{"%v":+%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.Error(t, err)

			data = fmt.Sprintf(`{"%v":-%v}`, key, value)

			err = Unmarshal([]byte(data), &jsonMap)
			require.Error(t, err)
		})
	})

	t.Run("MinKey(), with parens", func(t *testing.T) {
		value := "MinKey()"

		t.Run("single key", func(t *testing.T) {
			var jsonMap map[string]any

			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue)
		})

		t.Run("multiple keys", func(t *testing.T) {
			var jsonMap map[string]any

			key1, key2, key3 := "key1", "key2", "key3"
			data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
				key1, value, key2, value, key3, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue1, ok := jsonMap[key1].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue1)

			jsonValue2, ok := jsonMap[key2].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue2)

			jsonValue3, ok := jsonMap[key3].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue3)
		})

		t.Run("in array", func(t *testing.T) {
			var jsonMap map[string]any

			data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
				key, value, value, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonArray, ok := jsonMap[key].([]any)
			require.True(t, ok)

			for _, _jsonValue := range jsonArray {
				jsonValue, ok := _jsonValue.(MinKey)
				require.True(t, ok)
				assert.Equal(t, MinKey{}, jsonValue)
			}
		})

		t.Run("no signs", func(t *testing.T) {
			var jsonMap map[string]any

			value := "MinKey()"
			data := fmt.Sprintf(`{"%v":+%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.Error(t, err)

			data = fmt.Sprintf(`{"%v":-%v}`, key, value)

			err = Unmarshal([]byte(data), &jsonMap)
			require.Error(t, err)
		})

		t.Run("with whitespace", func(t *testing.T) {
			var jsonMap map[string]any

			value = "MinKey ( )"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(MinKey)
			require.True(t, ok)
			assert.Equal(t, MinKey{}, jsonValue)
		})

		t.Run("with something inside parens", func(t *testing.T) {
			var jsonMap map[string]any

			value = "MinKey(5)"
			data := fmt.Sprintf(`{"%v":%v}`, key, value)

			err := Unmarshal([]byte(data), &jsonMap)
			require.Error(t, err)
		})
	})
}
