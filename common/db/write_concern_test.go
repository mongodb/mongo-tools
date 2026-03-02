// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

func TestNewMongoWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("valid write concern on command line", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern(`{w:34}`, nil)
		require.NoError(t, err)
		assert.Equal(t, 34, writeConcern.W)

		writeConcern, err = NewMongoWriteConcern(`{w:"majority"}`, nil)
		require.NoError(t, err)
		assert.Equal(t, majString, writeConcern.W)

		writeConcern, err = NewMongoWriteConcern(`majority`, nil)
		require.NoError(t, err)
		assert.Equal(t, majString, writeConcern.W)

		writeConcern, err = NewMongoWriteConcern(`tagset`, nil)
		require.NoError(t, err)
		assert.Equal(t, "tagset", writeConcern.W)
	})

	t.Run("w=0 and no j", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern(`{w:0}`, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, writeConcern.W)
	})

	t.Run("negative w", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern(`{w:-1}`, nil)
		require.Error(t, err)
		assert.Nil(t, writeConcern)

		writeConcern, err = NewMongoWriteConcern(`{w:-2}`, nil)
		require.Error(t, err)
		assert.Nil(t, writeConcern)
	})

	t.Run("w=0 and j=true", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern(`{w:0, j:true}`, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, writeConcern.W)
		assert.True(t, *writeConcern.Journal)
	})

	// Regression test for TOOLS-1741
	t.Run("default to majority", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern("", nil)
		require.NoError(t, err)
		assert.Equal(t, majString, writeConcern.W)
	})

	t.Run("w=0 and no j in connstring", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern(
			"",
			&connstring.ConnString{WNumber: 0, WNumberSet: true},
		)
		require.NoError(t, err)
		assert.Equal(t, 0, writeConcern.W)
	})

	t.Run("negative w in connstring", func(t *testing.T) {
		_, err := NewMongoWriteConcern(
			"",
			&connstring.ConnString{WNumber: -1, WNumberSet: true},
		)
		require.Error(t, err)

		_, err = NewMongoWriteConcern(
			"",
			&connstring.ConnString{WNumber: -2, WNumberSet: true},
		)
		require.Error(t, err)
	})

	t.Run("command line arg wins", func(t *testing.T) {
		writeConcern, err := NewMongoWriteConcern(
			`{w: 4}`,
			&connstring.ConnString{WNumber: 0, WNumberSet: true},
		)
		require.NoError(t, err)
		assert.Equal(t, 4, writeConcern.W)
	})
}

func TestConstructWCFromConnString(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("non-integer values", func(t *testing.T) {
		writeConcernString := "majority"
		cs := &connstring.ConnString{
			WString: writeConcernString,
		}
		writeConcern, err := constructWCFromConnString(cs)
		require.NoError(t, err)
		assert.Equal(t, majString, writeConcern.W)
	})

	t.Run("int values", func(t *testing.T) {
		cs := &connstring.ConnString{
			WNumber:    4,
			WNumberSet: true,
		}
		writeConcern, err := constructWCFromConnString(cs)
		require.NoError(t, err)
		assert.Equal(t, 4, writeConcern.W)
	})

	t.Run("valid j and w", func(t *testing.T) {
		// Note: this used to test WTImeout as well, but the upgrade to Go driver v2 removed wtimeout
		// support from connstring parsing, so we can't/don't do it here any more.
		expectedW := 3
		cs := &connstring.ConnString{
			WNumber:    3,
			WNumberSet: true,
			J:          true,
		}
		writeConcern, err := constructWCFromConnString(cs)
		require.NoError(t, err)
		assert.Equal(t, expectedW, writeConcern.W)
		assert.True(t, *writeConcern.Journal)
	})

	t.Run("unacknowlededged write concern", func(t *testing.T) {
		cs := &connstring.ConnString{
			WNumber:    0,
			WNumberSet: true,
		}
		writeConcern, err := constructWCFromConnString(cs)
		require.NoError(t, err)
		assert.Equal(t, 0, writeConcern.W)
	})
}
