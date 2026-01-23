// Copyright (C) MongoDB, Inc. 2026-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestGetOpTimeFromRawOplogEntry_RoundTripEmpty(t *testing.T) {
	raw, err := bson.Marshal(Oplog{})
	require.NoError(t, err)

	optime, err := GetOpTimeFromRawOplogEntry(raw)
	require.NoError(t, err)

	assert.Equal(t, OpTime{}, optime)
}
