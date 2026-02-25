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

func TestNumberLongValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("NumberLong constructor", func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: json.NumberLong(42),
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, int64(42), jsonMap[key])
	})

	t.Run(`NumberLong document ('{ "$numberLong": "42" }')`, func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$numberLong": "42",
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)
		assert.Equal(t, int64(42), jsonMap[key])
	})
}
