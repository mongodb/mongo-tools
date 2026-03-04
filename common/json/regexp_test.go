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

func TestRegExpValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `RegExp("foo", "i")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "i"}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := `RegExp("foo", "i")`,
			`RegExp("bar", "i")`, `RegExp("baz", "i")`
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "i"}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"bar", "i"}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"baz", "i"}, jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `RegExp("xyz", "i")`
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(RegExp)
			require.True(t, ok)
			assert.Equal(t, RegExp{"xyz", "i"}, jsonValue)
		}
	})

	t.Run("with single option", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		options := []string{"g", "i", "m", "s"}

		for _, option := range options {
			data := fmt.Sprintf(`{"%v":RegExp("xyz", "%v")}`, key, option)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(RegExp)
			require.True(t, ok)
			assert.Equal(t, RegExp{"xyz", option}, jsonValue)
		}
	})

	t.Run("with multiple options", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `RegExp("foo", "gims")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "gims"}, jsonValue)
	})
}

func TestRegExpLiteral(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "/foo/i"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "i"}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "/foo/i", "/bar/i", "/baz/i"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "i"}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"bar", "i"}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"baz", "i"}, jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "/xyz/i"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(RegExp)
			require.True(t, ok)
			assert.Equal(t, RegExp{"xyz", "i"}, jsonValue)
		}
	})

	t.Run("with single option", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		options := []string{"g", "i", "m", "s"}

		for _, option := range options {
			data := fmt.Sprintf(`{"%v":/xyz/%v}`, key, option)

			err := Unmarshal([]byte(data), &jsonMap)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(RegExp)
			require.True(t, ok)
			assert.Equal(t, RegExp{"xyz", option}, jsonValue)
		}
	})

	t.Run("with multiple options", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "/foo/gims"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "gims"}, jsonValue)
	})

	t.Run("can contain unescaped quotes (`'` and `\"`)", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `/f'o"o/i`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{`f'o"o`, "i"}, jsonValue)
	})

	t.Run("unescaped forward slashes", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "/f/o/o/i"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})

	t.Run("invalid escape sequences", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `/f\o\o/`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
