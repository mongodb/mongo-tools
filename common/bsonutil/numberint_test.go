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
)

func TestNumberIntValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("NumberInt constructor", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: json.NumberInt(42),
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, int32(42), jsonMap[key])
	})

	t.Run(`NumberInt document ('{ "$numberInt": "42" }')`, func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$numberInt": "42",
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, int32(42), jsonMap[key])
	})
}
