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

func TestUnquotedKeys(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "value"
		data := fmt.Sprintf(`{%v:"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value, jsonMap[key])
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "value1", "value2", "value3"
		data := fmt.Sprintf(`{%v:"%v",%v:"%v",%v:"%v"}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value1, jsonMap[key1])
		assert.Equal(t, value2, jsonMap[key2])
		assert.Equal(t, value3, jsonMap[key3])
	})

	t.Run("start with dollar sign", func(t *testing.T) {
		var jsonMap map[string]any

		key := "$dollar"
		value := "money"
		data := fmt.Sprintf(`{%v:"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value, jsonMap[key])
	})

	t.Run("start with underscore", func(t *testing.T) {
		var jsonMap map[string]any

		key := "_id"
		value := "unique"
		data := fmt.Sprintf(`{%v:"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value, jsonMap[key])
	})

	t.Run("start with number", func(t *testing.T) {
		var jsonMap map[string]any

		key := "073"
		value := "octal"
		data := fmt.Sprintf(`{%v:"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})

	t.Run("contain numbers", func(t *testing.T) {
		var jsonMap map[string]any

		key := "b16"
		value := "little"
		data := fmt.Sprintf(`{%v:"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		assert.Equal(t, value, jsonMap[key])
	})

	t.Run("contain dot", func(t *testing.T) {
		var jsonMap map[string]any

		key := "horse.horse"
		value := "horse"
		data := fmt.Sprintf(`{%v:"%v"}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}

func TestUnquotedValues(t *testing.T) {
	t.Run("single value", func(t *testing.T) {
		var jsonMap map[string]any

		key := "key"
		value := "value"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})

	t.Run("multiple values", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value1, value2, value3 := "value1", "value2", "value3"
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value1, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
