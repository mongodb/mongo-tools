// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package intents

import (
	"container/heap"
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

func TestBasicDBHeapBehavior(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("dbCounters with different active counts", func(t *testing.T) {
		dbheap := &DBHeap{}
		heap.Init(dbheap)
		heap.Push(dbheap, &dbCounter{75, nil})
		heap.Push(dbheap, &dbCounter{121, nil})
		heap.Push(dbheap, &dbCounter{76, nil})
		heap.Push(dbheap, &dbCounter{51, nil})
		heap.Push(dbheap, &dbCounter{82, nil})
		heap.Push(dbheap, &dbCounter{117, nil})
		heap.Push(dbheap, &dbCounter{49, nil})
		heap.Push(dbheap, &dbCounter{101, nil})
		heap.Push(dbheap, &dbCounter{122, nil})
		heap.Push(dbheap, &dbCounter{33, nil})
		heap.Push(dbheap, &dbCounter{0, nil})

		// pop in active order, least to greatest
		prev := -1
		for dbheap.Len() > 0 {
			//nolint:errcheck // the heap only contains *dbCounter values
			popped := heap.Pop(dbheap).(*dbCounter)
			assert.Greater(t, popped.active, prev)
			prev = popped.active
		}
	})

	t.Run("dbCounters with different bson sizes", func(t *testing.T) {
		dbheap := &DBHeap{}
		heap.Init(dbheap)
		heap.Push(dbheap, &dbCounter{0, []*Intent{{Size: 70}}})
		heap.Push(dbheap, &dbCounter{0, []*Intent{{Size: 1024}}})
		heap.Push(dbheap, &dbCounter{0, []*Intent{{Size: 97}}})
		heap.Push(dbheap, &dbCounter{0, []*Intent{{Size: 3}}})
		heap.Push(dbheap, &dbCounter{0, []*Intent{{Size: 1024 * 1024}}})

		// pop in size order, greatest to least
		prev := int64(1024*1024 + 1) // Maximum
		for dbheap.Len() > 0 {
			//nolint:errcheck // the heap only contains *dbCounter values
			popped := heap.Pop(dbheap).(*dbCounter)
			assert.Less(t, popped.collections[0].Size, prev)
			prev = popped.collections[0].Size
		}
	})
}

func TestDBCounterCollectionSorting(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	dbc := &dbCounter{
		collections: []*Intent{
			{Size: 100},
			{Size: 1000},
			{Size: 1},
			{Size: 10},
		},
	}

	// popping should return in decreasing BSONSize
	dbc.SortCollectionsBySize()
	assert.EqualValues(t, 1000, dbc.PopIntent().Size)
	assert.EqualValues(t, 100, dbc.PopIntent().Size)
	assert.EqualValues(t, 10, dbc.PopIntent().Size)
	assert.EqualValues(t, 1, dbc.PopIntent().Size)
	assert.Nil(t, dbc.PopIntent())
	assert.Nil(t, dbc.PopIntent())
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

func TestSimulatedMultiDBJob(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	intents := []*Intent{
		{C: "small", DB: "db2", Size: 32},
		{C: "medium", DB: "db2", Size: 128},
		{C: "giant", DB: "db1", Size: 1024},
		{C: "tiny", DB: "db1", Size: 2},
	}
	prioritizer := newMultiDatabaseLTFPrioritizer(intents)
	require.NotNil(t, prioritizer)

	// We're simulating two job threads here.

	i0 := prioritizer.Get()
	require.NotNil(t, i0)
	assert.Equal(t, "giant", i0.C, "first intent collection is largest bson file")
	assert.Equal(t, "db1", i0.DB, "first intent db is largest bson file")

	i1 := prioritizer.Get()
	require.NotNil(t, i1)
	assert.Equal(t, "medium", i1.C, "second intent collection is largest bson file for db2")
	assert.Equal(t, "db2", i1.DB, "second intent db is largest bson file for db2")

	// second job finishes the smaller intent
	prioritizer.Finish(i1)

	i2 := prioritizer.Get()
	require.NotNil(t, i2)
	assert.Equal(t, "small", i2.C, "third intent collection is from db2")
	assert.Equal(t, "db2", i2.DB, "second intent db is from db2")
	prioritizer.Finish(i2)

	i3 := prioritizer.Get()
	require.NotNil(t, i3)
	assert.Equal(t, "tiny", i3.C, "final intent collection is from db1")
	assert.Equal(t, "db1", i3.DB, "final intent db is from db1")

	// we expect 2 active db1 jobs
	counter, ok := prioritizer.counterMap["db1"]
	require.True(t, ok)
	assert.Equal(t, 2, counter.active, "both db1 jobs are still running")

	assert.Zero(t, prioritizer.dbHeap.Len(), "prioritizer heap is empty")
}
