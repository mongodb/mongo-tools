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

func TestObjectIdValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `ObjectId("123")`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(ObjectId)
		require.True(t, ok)
		assert.Equal(t, ObjectId("123"), jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := `ObjectId("123")`, `ObjectId("456")`, `ObjectId("789")`
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(ObjectId)
		require.True(t, ok)
		assert.Equal(t, ObjectId("123"), jsonValue1)

		jsonValue2, ok := jsonMap[key2].(ObjectId)
		require.True(t, ok)
		assert.Equal(t, ObjectId("456"), jsonValue2)

		jsonValue3, ok := jsonMap[key3].(ObjectId)
		require.True(t, ok)
		assert.Equal(t, ObjectId("789"), jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `ObjectId("000")`
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(ObjectId)
			require.True(t, ok)
			assert.Equal(t, ObjectId("000"), jsonValue)
		}
	})

	t.Run("number arg", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := `ObjectId(123)`
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
