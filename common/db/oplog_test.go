// Copyright (C) MongoDB, Inc. 2026-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGetOpTimeFromRawOplogEntry_RoundTrip(t *testing.T) {
	ops := []Oplog{
		{},
		{
			Timestamp: primitive.Timestamp{1, 2},
		},
		{
			Timestamp: primitive.Timestamp{1, 2},
			Term:      lo.ToPtr(int64(234)),
		},
		{
			Timestamp: primitive.Timestamp{1, 2},
			Term:      lo.ToPtr(int64(234)),
			Hash:      lo.ToPtr(int64(345)),
		},
	}

	for _, op := range ops {
		raw, err := bson.Marshal(op)
		require.NoError(t, err)

		optime, err := GetOpTimeFromRawOplogEntry(raw)
		require.NoError(t, err)

		assert.Equal(t, GetOpTimeFromOplogEntry(&op), optime)
	}
}
