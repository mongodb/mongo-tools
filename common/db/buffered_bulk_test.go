// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"strings"
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

	t.Run("full buffer with max-length namespace", func(t *testing.T) {
		// Fill the buffer to byteLimit with small docs against the longest namespace
		// the server allows, and verify the resulting BulkWrite succeeds (i.e. the
		// 1 MB margin is sufficient to cover OP_MSG overhead).
		//
		// Sharded clusters enforce a 235-byte namespace limit; standalone/replica
		// sets allow 255 bytes. Using db "t" (1 char) leaves room for the dot, so
		// the collection name is (limit - 2) chars.
		longDB := "t"
		collLen := 253 // 1 + "." + 253 = 255-byte full namespace
		isMongos, err := provider.IsMongos()
		require.NoError(t, err)
		if isMongos {
			collLen = 233 // 1 + "." + 233 = 235-byte full namespace
		}
		longColl := strings.Repeat("x", collLen)

		defer func() {
			require.NoError(t, provider.DropDatabase(longDB))
		}()

		testCol := session.Database(longDB).Collection(longColl)
		bufBulk := NewUnorderedBufferedBulkInserter(testCol, 10000, serverVersion)
		require.NotNil(t, bufBulk)

		// Each doc is ~5 KB; after ~9500 buffered the next triggers a byteLimit flush.
		docPayload := strings.Repeat("y", 5000)
		var flushed bool
		for i := 0; i < 10000; i++ {
			result, insertErr := bufBulk.Insert(t.Context(), bson.M{"data": docPayload})
			require.NoError(t, insertErr)
			if result != nil {
				assert.Positive(t, result.InsertedCount)
				flushed = true
				break
			}
		}
		require.True(t, flushed, "expected buffer to flush within 10000 docs")

		_, flushErr := bufBulk.Flush(t.Context())
		require.NoError(t, flushErr)
	})

	t.Run("byte limit 1", func(t *testing.T) {
		testCol := session.Database("tools-test").Collection("bulk4")
		bufBulk := NewUnorderedBufferedBulkInserter(testCol, 1000, serverVersion)
		require.NotNil(t, bufBulk)
		bufBulk.byteLimit = 1

		for i := 0; i < 10; i++ {
			result, err := bufBulk.Insert(t.Context(), bson.D{{"foo", "bar"}})
			require.NoError(t, err)
			if i == 0 {
				// First insert: buffer was empty, no pre-flush needed.
				assert.Nil(t, result)
			} else {
				// Subsequent inserts: adding the doc would exceed byte limit,
				// so the previous doc is flushed before this one is buffered.
				require.NotNil(t, result)
				assert.EqualValues(t, 1, result.InsertedCount)
			}
		}
		// One doc remains in the buffer; flush it.
		result, err := bufBulk.Flush(t.Context())
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.EqualValues(t, 1, result.InsertedCount)
	})

}
