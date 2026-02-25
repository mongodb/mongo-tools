// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package archive

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type testConsumer struct {
	headers []string // header data
	bodies  []string // body data
	eof     bool
}

func (tc *testConsumer) HeaderBSON(b []byte) error {
	ss := strStruct{}
	err := bson.Unmarshal(b, &ss)
	tc.headers = append(tc.headers, ss.Str)
	return err
}

func (tc *testConsumer) BodyBSON(b []byte) error {
	ss := strStruct{}
	err := bson.Unmarshal(b, &ss)
	tc.bodies = append(tc.bodies, ss.Str)
	return err
}

func (tc *testConsumer) End() (err error) {
	if tc.eof {
		err = fmt.Errorf("double end")
	}
	tc.eof = true
	return err
}

type strStruct struct {
	Str string
}

var term = []byte{0xFF, 0xFF, 0xFF, 0xFF}
var notTerm = []byte{0xFF, 0xFF, 0xFF, 0xFE}

func TestParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// setup returns a parser + consumer + buffer to use for tests; the parser is set up to read from
	// the buffer.
	setup := func() (Parser, *testConsumer, *bytes.Buffer) {
		buf := new(bytes.Buffer)
		return Parser{In: buf}, new(testConsumer), buf
	}

	writeBSON := func(buf *bytes.Buffer, data any) {
		b, _ := bson.Marshal(data)
		buf.Write(b)
	}

	t.Run("well-formed header and body", func(t *testing.T) {
		parser, tc, buf := setup()
		writeBSON(buf, strStruct{"header"})
		writeBSON(buf, strStruct{"body"})
		buf.Write(term)

		t.Run("ReadBlock data parses correctly", func(t *testing.T) {
			err := parser.ReadBlock(tc)
			require.NoError(t, err)
			assert.False(t, tc.eof)
			assert.Equal(t, "header", tc.headers[0])
			assert.Equal(t, "body", tc.bodies[0])

			err = parser.ReadBlock(tc)
			require.ErrorIs(t, err, io.EOF)
		})

		t.Run("ReadAllBlock data parses correctly", func(t *testing.T) {
			err := parser.ReadAllBlocks(tc)
			require.NoError(t, err)
			assert.True(t, tc.eof)
			assert.Equal(t, "header", tc.headers[0])
			assert.Equal(t, "body", tc.bodies[0])
		})
	})

	t.Run("well-formed header and multiple bodies", func(t *testing.T) {
		parser, tc, buf := setup()
		writeBSON(buf, strStruct{"header"})
		writeBSON(buf, strStruct{"body0"})
		writeBSON(buf, strStruct{"body1"})
		writeBSON(buf, strStruct{"body2"})
		buf.Write(term)

		err := parser.ReadBlock(tc)
		require.NoError(t, err)
		assert.False(t, tc.eof)
		assert.Equal(t, "header", tc.headers[0])
		assert.Equal(t, "body0", tc.bodies[0])
		assert.Equal(t, "body1", tc.bodies[1])
		assert.Equal(t, "body2", tc.bodies[2])

		err = parser.ReadBlock(tc)
		require.ErrorIs(t, err, io.EOF)
		assert.False(t, tc.eof)
	})

	t.Run("incorrect terminator", func(t *testing.T) {
		parser, tc, buf := setup()
		writeBSON(buf, strStruct{"header"})
		writeBSON(buf, strStruct{"body"})
		buf.Write(notTerm)

		err := parser.ReadBlock(tc)
		require.Error(t, err)
	})

	t.Run("empty block", func(t *testing.T) {
		parser, tc, _ := setup()
		err := parser.ReadBlock(tc)
		require.ErrorIs(t, err, io.EOF)
		assert.False(t, tc.eof)
	})

	t.Run("error progagation from consumer through parser", func(t *testing.T) {
		parser, tc, _ := setup()
		tc.eof = true
		err := parser.ReadAllBlocks(tc)
		require.ErrorContains(t, err, "double end")
	})

	t.Run("partial block", func(t *testing.T) {
		parser, tc, buf := setup()
		writeBSON(buf, strStruct{"header"})
		writeBSON(buf, strStruct{"body"})

		err := parser.ReadBlock(tc)
		require.Error(t, err)
		assert.False(t, tc.eof)
		assert.Equal(t, "header", tc.headers[0])
		assert.Equal(t, "body", tc.bodies[0])
	})

	t.Run("block with missing terminator", func(t *testing.T) {
		parser, tc, buf := setup()
		writeBSON(buf, strStruct{"header"})

		b, _ := bson.Marshal(strStruct{"body"})
		buf.Write(b[:len(b)-1])
		buf.WriteByte(0x01)
		buf.Write(notTerm)

		err := parser.ReadBlock(tc)
		require.Error(t, err)
		assert.False(t, tc.eof)
		assert.Equal(t, "header", tc.headers[0])
		assert.Empty(t, tc.bodies)
	})
}
