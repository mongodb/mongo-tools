// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/dumprestore"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkipCollection(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	md := &MongoDump{
		OutputOptions: &OutputOptions{
			ExcludedCollections:        []string{"test", "fake"},
			ExcludedCollectionPrefixes: []string{"pre-", "no"},
		},
	}

	assert.True(t, md.shouldSkipCollection("pre-test"), "skip 'pre-test'")
	assert.True(t, md.shouldSkipCollection("notest"), "skip 'notest'")
	assert.True(t, md.shouldSkipCollection("test"), "skip 'test'")
	assert.True(t, md.shouldSkipCollection("fake"), "skip 'fake'")

	assert.False(t, md.shouldSkipCollection("fake222"), "do not skip 'fake222'")
	assert.False(t, md.shouldSkipCollection("random"), "do not skip 'random'")
	assert.False(t, md.shouldSkipCollection("mytest"), "do not skip 'mytest'")
	assert.False(t, md.shouldSkipCollection("prefix"), "do not skip 'prefix'")
}

type testTable struct {
	db       string
	coll     string
	output   bool
	dbOption string
}

func TestShouldSkipSystemNamespace(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	tests := []testTable{
		{
			db:     "test",
			coll:   "system",
			output: false,
		},
		{
			db:     "test",
			coll:   "system.nonsense",
			output: true,
		},
		{
			db:     "test",
			coll:   "system.indexBuilds",
			output: true,
		},
		{
			db:     "test",
			coll:   "system.js",
			output: false,
		},
		{
			db:     "test",
			coll:   "test",
			output: false,
		},
		{
			db:     "config",
			coll:   "transactions",
			output: true,
		},
		{
			db:     "config",
			coll:   "system.sessions",
			output: true,
		},
		{
			db:     "config",
			coll:   "transaction_coordinators",
			output: true,
		},
		{
			db:     "config",
			coll:   "system.indexBuilds",
			output: true,
		},
		{
			db:     "config",
			coll:   "image_collection",
			output: true,
		},
		{
			db:     "config",
			coll:   "mongos",
			output: true,
		},
		{
			db:     "config",
			coll:   "system.preimages",
			output: true,
		},
		{
			db:     "config",
			coll:   "system.sharding_ddl_coordinators",
			output: true,
		},
		{
			db:     "config",
			coll:   "cache.foo",
			output: true,
		},
		{
			db:     "config",
			coll:   "foo",
			output: true,
		},
		{
			db:     "config",
			coll:   "chunks",
			output: false,
		},
		{
			db:     "config",
			coll:   "collections",
			output: false,
		},
		{
			db:     "config",
			coll:   "databases",
			output: false,
		},
		{
			db:     "config",
			coll:   "settings",
			output: false,
		},
		{
			db:     "config",
			coll:   "shards",
			output: false,
		},
		{
			db:     "config",
			coll:   "tags",
			output: false,
		},
		{
			db:     "config",
			coll:   "version",
			output: false,
		},
		{
			db:       "config",
			coll:     "foo",
			output:   false,
			dbOption: "config",
		},
		{
			db:       "config",
			coll:     "chunks",
			output:   false,
			dbOption: "config",
		},
	}

	for _, collName := range dumprestore.ConfigCollectionsToKeep {
		tests = append(tests, testTable{
			db:     "config",
			coll:   collName,
			output: false,
		})
	}

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	for _, testVals := range tests {
		md.ToolOptions.DB = testVals.dbOption

		if md.shouldSkipSystemNamespace(testVals.db, testVals.coll) != testVals.output {
			t.Errorf(
				"%s.%s should have been %v but failed\n",
				testVals.db,
				testVals.coll,
				testVals.output,
			)
		}
	}
}
