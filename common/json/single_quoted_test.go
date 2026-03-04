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

func TestSingleQuotedKeys(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "value"
		data := fmt.Sprintf(`{'%v':"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value, jsonMap[key])
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "value1", "value2", "value3"
		data := fmt.Sprintf(`{'%v':"%v",'%v':"%v",'%v':"%v"}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value1, jsonMap[key1])
		assert.Equal(t, value2, jsonMap[key2])
		assert.Equal(t, value3, jsonMap[key3])
	})
}

func TestSingleQuotedValues(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single value", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "value"
		data := fmt.Sprintf(`{"%v":'%v'}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value, jsonMap[key])
	})

	t.Run("multiple values", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "value1", "value2", "value3"
		data := fmt.Sprintf(`{"%v":'%v',"%v":'%v',"%v":'%v'}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value1, jsonMap[key1])
		assert.Equal(t, value2, jsonMap[key2])
		assert.Equal(t, value3, jsonMap[key3])
	})

	t.Run("in BinData constructor", func(t *testing.T) {
		var jsonMap map[string]any

		key := "bindata"
		value := "BinData(1, 'xyz')"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(BinData)
		require.True(t, ok)
		assert.Equal(t, byte(1), jsonValue.Type)
		assert.Equal(t, "xyz", jsonValue.Base64)
	})

	t.Run("in Boolean constructor", func(t *testing.T) {
		var jsonMap map[string]any

		key := "boolean"
		value := "Boolean('xyz')"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(bool)
		require.True(t, ok)
		assert.Equal(t, true, jsonValue)
	})

	t.Run("in DBRef constructor", func(t *testing.T) {
		var jsonMap map[string]any

		key := "dbref"
		value := "DBRef('examples', 'xyz')"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(DBRef)
		require.True(t, ok)
		assert.Equal(t, "examples", jsonValue.Collection)
		assert.Equal(t, "xyz", jsonValue.Id)
		assert.Empty(t, jsonValue.Database)
	})

	t.Run("in ObjectId constructor", func(t *testing.T) {
		var jsonMap map[string]any

		key := "_id"
		value := "ObjectId('xyz')"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(ObjectId)
		require.True(t, ok)
		assert.Equal(t, ObjectId("xyz"), jsonValue)
	})

	t.Run("in RegExp constructor", func(t *testing.T) {
		var jsonMap map[string]any

		key := "regex"
		value := "RegExp('xyz', 'i')"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(RegExp)
		require.True(t, ok)
		assert.Equal(t, "xyz", jsonValue.Pattern)
		assert.Equal(t, "i", jsonValue.Options)
	})
}
