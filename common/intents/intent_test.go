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
)

func TestIntentManager(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	manager := NewIntentManager()
	require.NotNil(t, manager)

	t.Run("add bson intents", func(t *testing.T) {
		manager.Put(&Intent{DB: "1", C: "1", Location: "/b1/"})
		manager.Put(&Intent{DB: "1", C: "2", Location: "/b2/"})
		manager.Put(&Intent{DB: "1", C: "3", Location: "/b3/"})
		manager.Put(&Intent{DB: "2", C: "1", Location: "/b4/"})
		assert.Len(t, manager.intentsByDiscoveryOrder, 4)
		assert.Len(t, manager.intents, 4)
	})

	t.Run("matching metadata intents", func(t *testing.T) {
		manager.Put(&Intent{DB: "2", C: "1", MetadataLocation: "/4m/"})
		manager.Put(&Intent{DB: "1", C: "3", MetadataLocation: "/3m/"})
		manager.Put(&Intent{DB: "1", C: "1", MetadataLocation: "/1m/"})
		manager.Put(&Intent{DB: "1", C: "2", MetadataLocation: "/2m/"})

		assert.Len(t, manager.intentsByDiscoveryOrder, 4, "intents by discovery unchanged")
		assert.Len(t, manager.intents, 4, "intents unchanged")
	})

	// stash these before we pop them all off again
	intentsToAdd := manager.NormalIntents()

	t.Run("popping them returns in insert order", func(t *testing.T) {
		manager.Finalize(Legacy)
		it0 := manager.Pop()
		it1 := manager.Pop()
		it2 := manager.Pop()
		it3 := manager.Pop()
		it4 := manager.Pop()
		require.Nil(t, it4)

		assert.Equal(t,
			Intent{DB: "1", C: "1", Location: "/b1/", MetadataLocation: "/1m/"}, *it0)
		assert.Equal(t,
			Intent{DB: "1", C: "2", Location: "/b2/", MetadataLocation: "/2m/"}, *it1)
		assert.Equal(t,
			Intent{DB: "1", C: "3", Location: "/b3/", MetadataLocation: "/3m/"}, *it2)
		assert.Equal(t,
			Intent{DB: "2", C: "1", Location: "/b4/", MetadataLocation: "/4m/"}, *it3)
	})

	// Restore state from before previous test.
	manager = NewIntentManager()
	for _, intent := range intentsToAdd {
		manager.Put(intent)
	}

	t.Run("adding non-matching intents increases size", func(t *testing.T) {
		manager.Put(&Intent{DB: "7", C: "49", MetadataLocation: "/5/"})
		manager.Put(&Intent{DB: "27", C: "B", MetadataLocation: "/6/"})

		assert.Len(t, manager.intentsByDiscoveryOrder, 6)
		assert.Len(t, manager.intents, 6)
	})

	t.Run("using the Peek() method", func(t *testing.T) {
		peeked := manager.Peek()
		require.NotNil(t, peeked)
		assert.Equal(t, manager.intentsByDiscoveryOrder[0], peeked)

		peeked.DB = "SHINY NEW VALUE"
		assert.NotEqual(
			t,
			peeked,
			manager.intentsByDiscoveryOrder[0],
			"modifying the returned copy should not modify the original",
		)
	})
}
