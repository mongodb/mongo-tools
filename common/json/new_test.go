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

func TestNewKeyword(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("BinData", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `new BinData(1, "xyz")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{1, "xyz"}, jsonValue)
	})

	t.Run("Boolean", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `new Boolean(1)`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.True(t, jsonValue)

		key = "key"
		value = `new Boolean(0)`
		data = fmt.Sprintf(`{"%v":%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok = jsonMap[key].(bool)
		require.True(t, ok)
		assert.False(t, jsonValue)
	})

	t.Run("Date", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "new Date(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(Date)
		require.True(t, ok)
		assert.Equal(t, Date(123), jsonValue)
	})

	t.Run("DBRef", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `new BinData(1, "xyz")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(BinData)
		require.True(t, ok)
		assert.Equal(t, BinData{1, "xyz"}, jsonValue)
	})

	t.Run("NumberInt", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "new NumberInt(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(NumberInt)
		require.True(t, ok)
		assert.Equal(t, NumberInt(123), jsonValue)
	})

	t.Run("NumberLong", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "new NumberLong(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(NumberLong)
		require.True(t, ok)
		assert.Equal(t, NumberLong(123), jsonValue)
	})

	t.Run("ObjectId", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `new ObjectId("123")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(ObjectId)
		require.True(t, ok)
		assert.Equal(t, ObjectId("123"), jsonValue)
	})

	t.Run("RegExp", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `new RegExp("foo", "i")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, RegExp{"foo", "i"}, jsonValue)
	})

	t.Run("Timestamp", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "new Timestamp(123, 321)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(Timestamp)
		require.True(t, ok)
		assert.Equal(t, Timestamp{123, 321}, jsonValue)
	})

	t.Run("fail with literal", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		literals := []string{"null", "true", "false", "undefined",
			"NaN", "Infinity", "MinKey", "MaxKey"}

		for _, value := range literals {
			data := fmt.Sprintf(`{"%v":new %v}`, key, value)
			t.Run(value, func(t *testing.T) {
				err := Unmarshal([]byte(data), &jsonMap)
				require.Error(t, err)
			})
		}
	})

	t.Run("must have space", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "newDate(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})

	t.Run("cannot be chained", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		//nolint:dupword
		value := "new new Date(123)"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
