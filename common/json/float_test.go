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

func TestNumberFloatValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	key := "key"

	t.Run("convert to JSON NumberFloat value", func(t *testing.T) {
		var jsonMap map[string]any

		value := "5.5"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(float64)
		require.True(t, ok)
		assert.EqualValues(t, NumberFloat(5.5), jsonValue)
	})

	t.Run("decimal point with trailing zero", func(t *testing.T) {
		var jsonMap map[string]any

		value := "5.0"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(float64)
		require.True(t, ok)
		assert.EqualValues(t, NumberFloat(5.0), jsonValue)

		numFloat := NumberFloat(jsonValue)
		byteValue, err := numFloat.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, "5.0", string(byteValue))
	})

	t.Run("precision with large decimals", func(t *testing.T) {
		var jsonMap map[string]any

		value := "5.52342123"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(float64)
		require.True(t, ok)
		assert.EqualValues(t, NumberFloat(5.52342123), jsonValue)

		numFloat := NumberFloat(jsonValue)
		byteValue, err := numFloat.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, "5.52342123", string(byteValue))
	})

	t.Run("exponent values", func(t *testing.T) {
		var jsonMap map[string]any

		value := "5e+32"
		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(float64)
		require.True(t, ok)
		assert.EqualValues(t, NumberFloat(5e32), jsonValue)

		numFloat := NumberFloat(jsonValue)
		byteValue, err := numFloat.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, "5e+32", string(byteValue))
	})
}
