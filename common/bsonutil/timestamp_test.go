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

func TestTimestampValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	testTS := bson.Timestamp{T: 123456, I: 55}

	t.Run("Timestamp literal", func(t *testing.T) {
		jsonMap := map[string]any{
			"ts": json.Timestamp{123456, 55},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, testTS, jsonMap["ts"])
	})

	t.Run(`Timestamp document {"ts":{"$timestamp":{"t":123456, "i":55}}}`, func(t *testing.T) {
		jsonMap := map[string]any{
			"ts": map[string]any{
				"$timestamp": map[string]any{
					"t": 123456.0,
					"i": 55.0,
				},
			},
		}

		bsonMap, err := ConvertLegacyExtJSONValueToBSON(jsonMap)
		require.NoError(t, err)

		realMap, ok := bsonMap.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testTS, realMap["ts"])
	})
}
