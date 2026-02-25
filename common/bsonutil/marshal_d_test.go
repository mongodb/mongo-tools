// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMarshalDMarshalJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("valid bson.D", func(t *testing.T) {
		testD := bson.D{
			{"cool", "rad"},
			{"aaa", 543.2},
			{"I", 0},
			{"E", 0},
			{"map", bson.M{"1": 1, "2": "two"}},
		}

		asJSON, err := json.Marshal(MarshalD(testD))
		require.NoError(t, err)
		strJSON := string(asJSON)

		t.Run("order preservation", func(t *testing.T) {
			assert.Less(t, strings.Index(strJSON, "cool"), strings.Index(strJSON, "aaa"))
			assert.Less(t, strings.Index(strJSON, "aaa"), strings.Index(strJSON, "I"))
			assert.Less(t, strings.Index(strJSON, "I"), strings.Index(strJSON, "E"))
			assert.Less(t, strings.Index(strJSON, "E"), strings.Index(strJSON, "map"))
			assert.Equal(t, 5, strings.Count(strJSON, ","), 5) // 4 + 1 from internal map
		})

		t.Run("json parsing", func(t *testing.T) {
			var asMap bson.M
			err := json.Unmarshal(asJSON, &asMap)
			require.NoError(t, err)

			assert.EqualValues(t, "rad", asMap["cool"])
			assert.EqualValues(t, 543.2, asMap["aaa"])
			assert.EqualValues(t, 0, asMap["I"])
			assert.EqualValues(t, 0, asMap["E"])

			asMapMap, ok := asMap["map"].(map[string]any)
			require.True(t, ok)

			assert.EqualValues(t, 1, asMapMap["1"])
			assert.Equal(t, "two", asMapMap["2"])
		})

		t.Run("inside another map", func(t *testing.T) {
			_, err := json.Marshal(bson.M{"x": 0, "y": MarshalD(testD)})
			require.NoError(t, err, "putting it inside another map still usable by json.Marshal")
		})
	})

	t.Run("empty bson.D", func(t *testing.T) {
		testD := bson.D{}
		asJSON, err := json.Marshal(MarshalD(testD))

		t.Run("wrap with MarshalD", func(t *testing.T) {
			require.NoError(t, err)
			strJSON := string(asJSON)
			assert.Equal(t, "{}", strJSON)
		})

		t.Run("json parsing", func(t *testing.T) {
			var asInterface any
			err := json.Unmarshal(asJSON, &asInterface)
			require.NoError(t, err)
			asMap, ok := asInterface.(map[string]any)
			require.True(t, ok)
			assert.Empty(t, asMap)
		})
	})
}

func TestFindValueByKey(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	subDocument := &bson.D{
		bson.E{Key: "field4", Value: "c"},
	}
	document := &bson.D{
		bson.E{Key: "field1", Value: "a"},
		bson.E{Key: "field2", Value: "b"},
		bson.E{Key: "field3", Value: subDocument},
	}

	value, err := FindValueByKey("field1", document)
	require.NoError(t, err)
	assert.Equal(t, "a", value, "get top-level key")

	value, err = FindValueByKey("field3", document)
	require.NoError(t, err)
	assert.Equal(t, subDocument, value, "top-lebel key with sub-document value")

	value, err = FindValueByKey("field4", document)
	require.Error(t, err)
	assert.Nil(t, value, "for non-existent key")
}

func TestEscapedKey(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	document := bson.D{
		bson.E{Key: `foo"bar`, Value: "a"},
	}

	asJSON, err := json.Marshal(MarshalD(document))
	require.NoError(t, err)

	var asMap bson.M
	err = json.Unmarshal(asJSON, &asMap)
	require.NoError(t, err)
	assert.Equal(t, "a", asMap[`foo"bar`], "original value correctly found with unescaped key")
}
