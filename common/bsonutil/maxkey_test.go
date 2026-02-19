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

func TestMaxKeyValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("MaxKey literal", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: json.MaxKey{},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, bson.MaxKey{}, jsonMap[key])
	})

	t.Run(`MaxKey document ('{ "$maxKey": 1 }')`, func(t *testing.T) {
		key := "maxKey"
		jsonMap := map[string]any{
			key: map[string]any{
				"$maxKey": 1,
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, bson.MaxKey{}, jsonMap[key])
	})
}
