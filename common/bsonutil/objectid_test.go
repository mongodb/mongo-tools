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

func TestObjectIdValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	oid, _ := bson.ObjectIDFromHex("0123456789abcdef01234567")

	t.Run("ObjectId constructor", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: json.ObjectId("0123456789abcdef01234567"),
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, oid, jsonMap[key])
	})

	t.Run(`ObjectId document ('{ "$oid": "0123456789abcdef01234567" }')`, func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$oid": "0123456789abcdef01234567",
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, oid, jsonMap[key])
	})
}
