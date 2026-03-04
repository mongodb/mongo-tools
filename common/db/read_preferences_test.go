// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/tag"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

func TestNewReadPreference(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tagSet := map[string]string{
		"foo": "bar",
	}
	cs := &connstring.ConnString{
		ReadPreference:        "secondary",
		ReadPreferenceTagSets: []map[string]string{tagSet},
		MaxStaleness:          time.Duration(5) * time.Second,
		MaxStalenessSet:       true,
	}

	t.Run("default to primary", func(t *testing.T) {
		pref, err := NewReadPreference("", nil)
		require.NoError(t, err)
		assert.Equal(t, readpref.PrimaryMode, pref.Mode())
	})

	t.Run("mode in command line", func(t *testing.T) {
		rp := "primary"
		pref, err := NewReadPreference(rp, nil)
		require.NoError(t, err)
		assert.Equal(t, readpref.PrimaryMode, pref.Mode())

		rp = "secondary"
		pref, err = NewReadPreference(rp, nil)
		require.NoError(t, err)
		assert.Equal(t, readpref.SecondaryMode, pref.Mode())

		rp = "nearest"
		pref, err = NewReadPreference(rp, nil)
		require.NoError(t, err)
		assert.Equal(t, readpref.NearestMode, pref.Mode())
	})

	t.Run("pref only on command line", func(t *testing.T) {
		rp := `{"mode": "secondary", "tagSets": [{"foo": "bar"}], maxStalenessSeconds: 123}`
		pref, err := NewReadPreference(rp, nil)
		require.NoError(t, err)
		assert.Equal(t, readpref.SecondaryMode, pref.Mode())

		tagSets := pref.TagSets()
		assert.Len(t, tagSets, 1)
		assert.Equal(t, tag.Set{tag.Tag{Name: "foo", Value: "bar"}}, tagSets[0])

		maxStaleness, set := pref.MaxStaleness()
		require.True(t, set)
		assert.Equal(t, 123*time.Second, maxStaleness)
	})

	t.Run("pref only in URI", func(t *testing.T) {
		pref, err := NewReadPreference("", cs)
		require.NoError(t, err)
		assert.Equal(t, readpref.SecondaryMode, pref.Mode())

		tagSets := pref.TagSets()
		assert.Len(t, tagSets, 1)
		assert.Equal(t, tag.Set{tag.Tag{Name: "foo", Value: "bar"}}, tagSets[0])

		maxStaleness, set := pref.MaxStaleness()
		require.True(t, set)
		assert.Equal(t, 5*time.Second, maxStaleness)
	})

	t.Run("pref in command line and URI", func(t *testing.T) {
		rp := `{"mode": "nearest", "tagSets": [{"one": "two"}], maxStalenessSeconds: 123}`
		pref, err := NewReadPreference(rp, cs)
		require.NoError(t, err)
		assert.Equal(t, readpref.NearestMode, pref.Mode())

		// command-line wins
		tagSets := pref.TagSets()
		assert.Len(t, tagSets, 1)
		assert.Equal(t, tag.Set{tag.Tag{Name: "one", Value: "two"}}, tagSets[0])

		maxStaleness, set := pref.MaxStaleness()
		require.True(t, set)
		assert.Equal(t, 123*time.Second, maxStaleness)
	})
}
