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

func TestDecodeBsonD(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("order preservation", func(t *testing.T) {
		data := `{"a":1, "b":2, "c":3, "d":4, "e":5, "f":6}`
		out := bson.D{}
		err := Unmarshal([]byte(data), &out)
		require.NoError(t, err)
		assert.Len(t, out, 6)
		assert.Equal(t, "a", out[0].Key)
		assert.Equal(t, "b", out[1].Key)
		assert.Equal(t, "c", out[2].Key)
		assert.Equal(t, "d", out[3].Key)
		assert.Equal(t, "e", out[4].Key)
		assert.Equal(t, "f", out[5].Key)

	})

	t.Run("nested documents", func(t *testing.T) {
		data := `{"a": 17, "b":{"foo":"bar", "baz":"boo"}, c:"wow" }`
		out := struct {
			A int    `json:"a"`
			B bson.D `json:"b"`
			C string `json:"c"`
		}{}
		err := Unmarshal([]byte(data), &out)
		require.NoError(t, err)
		assert.Equal(t, 17, out.A)
		assert.Equal(t, "wow", out.C)
		assert.Len(t, out.B, 2)
		assert.Equal(t, "foo", out.B[0].Key)
		assert.Equal(t, "bar", out.B[0].Value)
		assert.Equal(t, "baz", out.B[1].Key)
		assert.Equal(t, "boo", out.B[1].Value)
	})

	t.Run("nested in DocElems", func(t *testing.T) {
		data := `{"a":["x", "y","z"], "b":{"foo":"bar", "baz":"boo"}}`
		out := bson.D{}
		err := Unmarshal([]byte(data), &out)
		require.NoError(t, err)
		assert.Len(t, out, 2)
		assert.Equal(t, "a", out[0].Key)
		assert.Equal(t, "b", out[1].Key)
		assert.Equal(t, []any{"x", "y", "z"}, out[0].Value)
		assert.Equal(t, bson.D{{"foo", "bar"}, {"baz", "boo"}}, out[1].Value)
	})

	t.Run("subdocuments inside bson.D", func(t *testing.T) {
		data := `{subA: {a:{b:{c:9}}}, subB:{a:{b:{c:9}}}}`
		out := struct {
			A any    `json:"subA"`
			B bson.D `json:"subB"`
		}{}
		err := Unmarshal([]byte(data), &out)
		require.NoError(t, err)
		//nolint:errcheck // this will always be a map[string]any
		aMap := out.A.(map[string]any)
		assert.Len(t, aMap, 1)
		//nolint:errcheck // this will always be a map[string]any
		aMapSub := aMap["a"].(map[string]any)
		assert.Len(t, aMapSub, 1)
		//nolint:errcheck // this will always be a map[string]any
		aMapSubSub := aMapSub["b"].(map[string]any)
		assert.EqualValues(t, 9, aMapSubSub["c"])
		assert.Len(t, out.B, 1)
		// using string comparison for simplicity
		c := bson.D{{Key: "c", Value: 9}}
		b := bson.D{{Key: "b", Value: c}}
		a := bson.D{{Key: "a", Value: b}}
		assert.Equal(t, fmt.Sprintf("%v", a), fmt.Sprintf("%v", out.B))
	})

	t.Run("subdocuments inside arrays", func(t *testing.T) {
		data := `{"a":[1,2,{b:"inner"}]}`
		out := bson.D{}
		err := Unmarshal([]byte(data), &out)
		require.NoError(t, err)
		assert.Len(t, out, 1)

		innerArray, ok := out[0].Value.([]any)
		require.True(t, ok)

		assert.Len(t, innerArray, 3)
		assert.EqualValues(t, 1, innerArray[0])
		assert.EqualValues(t, 2, innerArray[1])

		innerD, ok := innerArray[2].(bson.D)
		require.True(t, ok)

		assert.Len(t, innerD, 1)
		assert.Equal(t, "b", innerD[0].Key)
		assert.Equal(t, "inner", innerD[0].Value)
	})

	t.Run("null is valid", func(t *testing.T) {
		data := `{"a":true, "b":null, "c": 5}`
		out := bson.D{}
		err := Unmarshal([]byte(data), &out)
		require.NoError(t, err)
		assert.Len(t, out, 3)
		assert.Equal(t, "a", out[0].Key)
		assert.Equal(t, true, out[0].Value)
		assert.Equal(t, "b", out[1].Key)
		assert.Nil(t, out[1].Value)
		assert.Equal(t, "c", out[2].Key)
		assert.EqualValues(t, 5, out[2].Value)
	})

	t.Run("non-bson.D slice types", func(t *testing.T) {
		data := `{"a":["x", "y","z"], "b":{"foo":"bar", "baz":"boo"}}`
		out := []any{}
		err := Unmarshal([]byte(data), &out)
		require.Error(t, err)
	})
}
