// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rfc3339Milli = "2006-01-02T15:04:05.999Z07:00"

func TestDateValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dates := []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
	}

	for _, dateString := range dates {
		example := fmt.Sprintf(`{ "$date": "%v" }`, dateString)
		t.Run(fmt.Sprintf("of string ('%v')", example), func(t *testing.T) {
			key := "key"
			jsonMap := map[string]any{
				key: map[string]any{
					"$date": dateString,
				},
			}

			err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
			require.NoError(t, err)

			// dateString is a valid time format string
			date, err := time.Parse(rfc3339Milli, dateString)
			require.NoError(t, err)

			jsonValue, ok := jsonMap[key].(time.Time)
			assert.True(t, ok)
			assert.Equal(t, date, jsonValue)
		})
	}

	date := time.Unix(0, int64(time.Duration(1136214245000)*time.Millisecond))

	t.Run(`of $numberLong ('{ "$date": { "$numberLong": "1136214245000" } }')`, func(t *testing.T) {
		key := "key"
		jsonMap := map[string]any{
			key: map[string]any{
				"$date": map[string]any{
					"$numberLong": "1136214245000",
				},
			},
		}

		err := ConvertLegacyExtJSONDocumentToBSON(jsonMap)
		require.NoError(t, err)

		jsonValue, ok := jsonMap[key].(time.Time)
		assert.True(t, ok)
		assert.Equal(t, date, jsonValue)
	})
}
