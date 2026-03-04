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
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDBPointerValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	oid, _ := bson.ObjectIDFromHex("552ffe9f5739878e73d116a9")
	oid2, _ := bson.ObjectIDFromHex("552ffed95739878e73d116aa")
	oid3, _ := bson.ObjectIDFromHex("552fff215739878e73d116ab")

	key := "key"
	value := `DBPointer("ref", ObjectId("552ffe9f5739878e73d116a9"))`

	t.Run("single key", func(t *testing.T) {
		var jsonMap map[string]any

		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(DBPointer)
		require.True(t, ok)

		assert.Equal(t, DBPointer{"ref", oid}, jsonValue)
	})

	t.Run("multiple keys", func(t *testing.T) {
		var jsonMap map[string]any

		key1, key2, key3 := "key1", "key2", "key3"
		value2 := `DBPointer("ref2", ObjectId("552ffed95739878e73d116aa"))`
		value3 := `DBPointer("ref3", ObjectId("552fff215739878e73d116ab"))`
		data := fmt.Sprintf(`{"%v":%v,"%v":%v,"%v":%v}`,
			key1, value, key2, value2, key3, value3)

		err := Unmarshal([]byte(data), &jsonMap)
		require.NoError(t, err)

		jsonValue1, ok := jsonMap[key1].(DBPointer)
		require.True(t, ok)

		assert.Equal(t, DBPointer{"ref", oid}, jsonValue1)

		jsonValue2, ok := jsonMap[key2].(DBPointer)
		require.True(t, ok)
		assert.Equal(t, DBPointer{"ref2", oid2}, jsonValue2)

		jsonValue3, ok := jsonMap[key3].(DBPointer)
		require.True(t, ok)
		assert.Equal(t, DBPointer{"ref3", oid3}, jsonValue3)
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
			jsonValue, ok := _jsonValue.(DBPointer)
			require.True(t, ok)
			assert.Equal(t, DBPointer{"ref", oid}, jsonValue)
		}
	})

	t.Run("not ObjectId", func(t *testing.T) {
		value := `DBPointer("ref", 4)`
		var jsonMap map[string]any

		data := fmt.Sprintf(`{"%v":%v}`, key, value)

		err := Unmarshal([]byte(data), &jsonMap)
		require.Error(t, err)
	})
}
