// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"bytes"
	"io"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBufferlessBSONSource(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	var testValues = []bson.M{
		{"_": bson.Binary{Subtype: 0x80, Data: []byte("apples")}},
		{"_": bson.Binary{Subtype: 0x80, Data: []byte("bananas")}},
		{"_": bson.Binary{Subtype: 0x80, Data: []byte("cherries")}},
	}

	writeBuf := bytes.NewBuffer(make([]byte, 0, 1024))
	for _, tv := range testValues {
		data, err := bson.Marshal(&tv)
		require.NoError(t, err)
		_, err = writeBuf.Write(data)
		require.NoError(t, err)
	}

	// Now, parse that buffer.
	bsonSource := NewDecodedBSONSource(NewBufferlessBSONSource(io.NopCloser(writeBuf)))
	docs := []bson.M{}
	count := 0
	doc := &bson.M{}
	for bsonSource.Next(doc) {
		count++
		docs = append(docs, *doc)
		doc = &bson.M{}
	}
	require.NoError(t, bsonSource.Err())
	assert.Equal(t, len(testValues), count)
	assert.Equal(t, testValues, docs)
}
