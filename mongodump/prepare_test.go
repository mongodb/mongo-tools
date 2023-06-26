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
	. "github.com/smartystreets/goconvey/convey"
)

func TestSkipCollection(t *testing.T) {

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a mongodump that excludes collections 'test' and 'fake'"+
		" and excludes prefixes 'pre-' and 'no'", t, func() {
		md := &MongoDump{
			OutputOptions: &OutputOptions{
				ExcludedCollections:        []string{"test", "fake"},
				ExcludedCollectionPrefixes: []string{"pre-", "no"},
			},
		}

		Convey("collection 'pre-test' should be skipped", func() {
			So(md.shouldSkipCollection("pre-test"), ShouldBeTrue)
		})

		Convey("collection 'notest' should be skipped", func() {
			So(md.shouldSkipCollection("notest"), ShouldBeTrue)
		})

		Convey("collection 'test' should be skipped", func() {
			So(md.shouldSkipCollection("test"), ShouldBeTrue)
		})

		Convey("collection 'fake' should be skipped", func() {
			So(md.shouldSkipCollection("fake"), ShouldBeTrue)
		})

		Convey("collection 'fake222' should not be skipped", func() {
			So(md.shouldSkipCollection("fake222"), ShouldBeFalse)
		})

		Convey("collection 'random' should not be skipped", func() {
			So(md.shouldSkipCollection("random"), ShouldBeFalse)
		})

		Convey("collection 'mytest' should not be skipped", func() {
			So(md.shouldSkipCollection("mytest"), ShouldBeFalse)
		})
	})

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

	md := simpleMongoDumpInstance()

	for _, testVals := range tests {
		md.ToolOptions.DB = testVals.dbOption

		if md.shouldSkipSystemNamespace(testVals.db, testVals.coll) != testVals.output {
			t.Errorf("%s.%s should have been %v but failed\n", testVals.db, testVals.coll, testVals.output)
		}
	}
}
