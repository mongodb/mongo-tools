// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package intents

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLegacyPrioritizer(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	testList := []*Intent{
		{DB: "1"},
		{DB: "2"},
		{DB: "3"},
	}
	legacy := newLegacyPrioritizer(testList)
	require.NotNil(t, legacy)

	// priority is first-in-first-out
	it0 := legacy.Get()
	it1 := legacy.Get()
	it2 := legacy.Get()
	it3 := legacy.Get()
	assert.Nil(t, it3)
	assert.Less(t, it0.DB, it1.DB)
	assert.Less(t, it1.DB, it2.DB)
}

func TestBySizeAndView(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	intents := []*Intent{
		{C: "non-view2", Size: 32},
		{C: "view", Size: 0,
			Options: bson.D{{Key: "viewOn", Value: true}},
			Type:    "view",
		},
		{C: "non-view1", Size: 1024},
		{C: "non-view3", Size: 2},
		{C: "view", Size: 0,
			Options: bson.D{{Key: "viewOn", Value: true}},
			Type:    "view",
		},
	}

	prioritizer := newLongestTaskFirstPrioritizer(intents)
	// views first, followed by collections largest to smallest

	assert.Equal(t, "view", prioritizer.Get().C)
	assert.Equal(t, "view", prioritizer.Get().C)
	assert.Equal(t, "non-view1", prioritizer.Get().C)
	assert.Equal(t, "non-view2", prioritizer.Get().C)
	assert.Equal(t, "non-view3", prioritizer.Get().C)

}
