// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestRegExpValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("works for RegExp constructor", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: json.RegExp{"foo", "i"},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, bson.Regex{"foo", "i"}, jsonMap[key])
	})

	t.Run("works for RegExp document", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$regex":   "foo",
				"$options": "i",
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(
			t,
			bson.Regex{"foo", "i"},
			jsonMap[key],
			`regex: { "$regex": "foo", "$options": "i" }`,
		)
	})

	t.Run("can use multiple options", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$regex":   "bar",
				"$options": "gims",
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(
			t,
			bson.Regex{"bar", "gims"},
			jsonMap[key],
			`regex: { "$regex": "bar", "$options": "gims" }`,
		)
	})

	t.Run("fails for an invalid option", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$regex":   "baz",
				"$options": "y",
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.Error(
			t,
			err,
			`regex: { "$regex": "baz", "$options": "y" }`,
		)
	})
}
