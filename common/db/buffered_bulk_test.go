// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBufferedBulkInserterInserts(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()
	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port: DefaultTestPort,
		},
		URI:  &options.URI{},
		SSL:  &ssl,
		Auth: &auth,
	}
	err := opts.NormalizeOptionsAndURI()
	require.NoError(t, err)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)
	require.NotNil(t, provider)

	defer provider.Close()

	session, err := provider.GetSession()
	require.NoError(t, err)
	require.NotNil(t, session)

	serverVersion, err := provider.ServerVersionArray()
	require.NoError(t, err)

	defer func() {
		require.NoError(t, provider.DropDatabase("tools-test"))
	}()

	t.Run("doc limit 3", func(t *testing.T) {
		testCol := session.Database("tools-test").Collection("bulk1")
		bufBulk := NewUnorderedBufferedBulkInserter(testCol, 3, serverVersion)
		require.NotNil(t, bufBulk)

		flushCount := 0
		for i := 0; i < 10; i++ {
			result, err := bufBulk.Insert(t.Context(), bson.D{})
			require.NoError(t, err)
			if bufBulk.docCount%3 == 0 {
				flushCount++
				require.NotNil(t, result)
				assert.EqualValues(t, 3, result.InsertedCount)
			} else {
				assert.Nil(t, result)
			}
		}

		assert.Equal(t, 3, flushCount)
		assert.Equal(t, 1, bufBulk.docCount)
	})

	t.Run("doc limit 1", func(t *testing.T) {
		testCol := session.Database("tools-test").Collection("bulk2")
		bufBulk := NewUnorderedBufferedBulkInserter(testCol, 1, serverVersion)
		require.NotNil(t, bufBulk)

		for i := 0; i < 10; i++ {
			result, err := bufBulk.Insert(t.Context(), bson.D{})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.EqualValues(t, 1, result.InsertedCount)
		}
		result, err := bufBulk.Flush(t.Context())
		require.NoError(t, err)
		assert.Nil(t, result)

		assert.Zero(t, bufBulk.docCount)
	})

	t.Run("doc limit 1000", func(t *testing.T) {
		require.NoError(t, provider.DropDatabase("tools-test"))

		testCol := session.Database("tools-test").Collection("bulk3")
		bufBulk := NewUnorderedBufferedBulkInserter(testCol, 100, serverVersion)
		require.NotNil(t, bufBulk)

		errCnt := 0
		for i := 0; i < 1_000_000; i++ {
			result, err := bufBulk.Insert(t.Context(), bson.M{"_id": i})
			if err != nil {
				errCnt++
			}
			if (i+1)%10000 == 0 {
				require.NotNil(t, result)
				assert.EqualValues(t, 100, result.InsertedCount)
			}
		}
		require.Zero(t, errCnt)

		_, err := bufBulk.Flush(t.Context())
		require.NoError(t, err)

		t.Run("check docs", func(t *testing.T) {
			count, err := testCol.CountDocuments(t.Context(), bson.M{})
			require.NoError(t, err)
			assert.EqualValues(t, 1_000_000, count)

			// test values
			testDoc := bson.M{}
			result := testCol.FindOne(t.Context(), bson.M{"_id": 477232})
			err = result.Decode(&testDoc)
			require.NoError(t, err)
			assert.EqualValues(t, 477232, testDoc["_id"])

			result = testCol.FindOne(t.Context(), bson.M{"_id": 999999})
			err = result.Decode(&testDoc)
			require.NoError(t, err)
			assert.EqualValues(t, 999999, testDoc["_id"])

			result = testCol.FindOne(t.Context(), bson.M{"_id": 1})
			err = result.Decode(&testDoc)
			require.NoError(t, err)
			assert.EqualValues(t, 1, testDoc["_id"])
		})
	})

	t.Run("byte limit 1", func(t *testing.T) {
		testCol := session.Database("tools-test").Collection("bulk4")
		bufBulk := NewUnorderedBufferedBulkInserter(testCol, 1000, serverVersion)
		require.NotNil(t, bufBulk)
		bufBulk.byteLimit = 1

		for i := 0; i < 10; i++ {
			result, err := bufBulk.Insert(t.Context(), bson.D{{"foo", "bar"}})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.EqualValues(t, 1, result.InsertedCount)
		}
	})

}
