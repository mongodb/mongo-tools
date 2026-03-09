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

func TestUndefinedValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "undefined"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(Undefined)
		require.True(t, ok)
		assert.Equal(t, Undefined{}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value := "undefined"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value, key2, value, key3, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(Undefined)
		require.True(t, ok)
		assert.Equal(t, Undefined{}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(Undefined)
		require.True(t, ok)
		assert.Equal(t, Undefined{}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(Undefined)
		require.True(t, ok)
		assert.Equal(t, Undefined{}, jsonValue3)
	})

	t.Run("in array", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "undefined"
		data := fmt.Sprintf(`{"%v":[%v,%v,%v]}`,
			key, value, value, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonArray, ok := jsonMap[key].([]any)
		require.True(t, ok)

		for _, _jsonValue := range jsonArray {
			jsonValue, ok := _jsonValue.(Undefined)
			require.True(t, ok)
			assert.Equal(t, Undefined{}, jsonValue)
		}
	})

	t.Run("with signs", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "undefined"
		data := fmt.Sprintf(`{"%v":+%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)

		data = fmt.Sprintf(`{"%v":-%v}`, key, value)

		err = Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
