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

func TestMinKeyValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("MinKey literal", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: json.MinKey{},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, bson.MinKey{}, jsonMap[key])
	})

	t.Run(`MinKey document ('{ "$minKey": 1 }')`, func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$minKey": 1,
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, bson.MinKey{}, jsonMap[key])
	})
}
