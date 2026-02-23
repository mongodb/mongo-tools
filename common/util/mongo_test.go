// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"fmt"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitHostArg(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("empty host", func(t *testing.T) {
		hosts, setName := SplitHostArg("")
		assert.Equal(t, []string{""}, hosts, "empty host slice")
		assert.Equal(t, "", setName, "empty replica set name")
	})

	t.Run("trailing slash", func(t *testing.T) {
		hosts, setName := SplitHostArg("foo/")
		assert.Equal(t, []string{""}, hosts, "empty host slice")
		assert.Equal(t, "foo", setName, "replica set name")
	})

	t.Run("host arg with no replica set name", func(t *testing.T) {
		hosts, setName := SplitHostArg("host1,host2")
		assert.Equal(t, []string{"host1", "host2"}, hosts, "host slice")
		assert.Equal(t, "", setName, "empty replica set name")
	})

	t.Run("host arg with replica set name", func(t *testing.T) {
		hosts, setName := SplitHostArg("foo/host1,host2")
		assert.Equal(t, []string{"host1", "host2"}, hosts, "host slice")
		assert.Equal(t, "foo", setName, "replica set name")
	})

	t.Run("host arg order", func(t *testing.T) {
		hosts, setName := SplitHostArg("foo/host2,host1")
		assert.Equal(t, []string{"host2", "host1"}, hosts, "host slice")
		assert.Equal(t, "foo", setName, "replica set name")
	})

	t.Run("host+port pairs", func(t *testing.T) {
		hosts, setName := SplitHostArg("foo/host1:27017,host2:27017")
		assert.Equal(t, []string{"host1:27017", "host2:27017"}, hosts, "host slice")
		assert.Equal(t, "foo", setName, "replica set name")
	})
}

func TestCreateConnectionAddrs(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("with no port", func(t *testing.T) {
		addrs := CreateConnectionAddrs("host1,host2", "")
		assert.Equal(t, []string{"host1", "host2"}, addrs, "no port in hosts")
	})

	t.Run("with port", func(t *testing.T) {
		addrs := CreateConnectionAddrs("host1,host2", "20000")
		assert.Equal(t, []string{"host1:20000", "host2:20000"}, addrs, "port appended to hosts")
	})

}

func TestBuildURI(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct{ h, p, u string }{
		{h: "", p: "", u: "mongodb://localhost/"},
		{h: "host1", p: "", u: "mongodb://host1/"},
		{h: "", p: "33333", u: "mongodb://localhost:33333/"},
		{h: "host1", p: "33333", u: "mongodb://host1:33333/"},
		{h: "host1,host2", p: "33333", u: "mongodb://host1:33333,host2:33333/"},
		{h: "host1,host2:27017", p: "33333", u: "mongodb://host1:33333,host2:27017/"},
		{h: "foo/", p: "", u: "mongodb://localhost/?replicaSet=foo"},
		{h: "foo/", p: "33333", u: "mongodb://localhost:33333/?replicaSet=foo"},
		{
			h: "foo/host1,host2:27017",
			p: "33333",
			u: "mongodb://host1:33333,host2:27017/?replicaSet=foo",
		},
	}

	for _, c := range cases {
		label := fmt.Sprintf("'%s', '%s'", c.h, c.p)
		t.Run(label, func(t *testing.T) {
			got := BuildURI(c.h, c.p)
			assert.Equal(t, c.u, got)
		})
	}
}

func TestInvalidNames(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("test.col$ is invalid", func(t *testing.T) {
		require.NoError(t, ValidateDBName("test"))
		require.Error(t, ValidateCollectionName("col$"))
		require.Error(t, ValidateFullNamespace("test.col$"))
	})

	t.Run("db/aaa.col is invalid", func(t *testing.T) {
		require.Error(t, ValidateDBName("db/aaa"))
		require.NoError(t, ValidateCollectionName("col"))
		require.Error(t, ValidateFullNamespace("db/aaa.col"))
	})
	t.Run("db. is invalid", func(t *testing.T) {
		require.NoError(t, ValidateDBName("db"))
		require.Error(t, ValidateCollectionName(""))
		require.Error(t, ValidateFullNamespace("db."))
	})
	t.Run("db space.col is invalid", func(t *testing.T) {
		require.Error(t, ValidateDBName("db space"))
		require.NoError(t, ValidateCollectionName("col"))
		require.Error(t, ValidateFullNamespace("db space.col"))
	})
	t.Run("db x$x is invalid", func(t *testing.T) {
		require.Error(t, ValidateDBName("x$x"))
		require.Error(t, ValidateFullNamespace("x$x.y"))
	})
	t.Run("[null].[null] is invalid", func(t *testing.T) {
		require.Error(t, ValidateDBName("\x00"))
		require.Error(t, ValidateCollectionName("\x00"))
		require.Error(t, ValidateFullNamespace("\x00.\x00"))
	})
	t.Run("[empty] is invalid", func(t *testing.T) {
		require.Error(t, ValidateFullNamespace(""))
	})
	t.Run("db.col is valid", func(t *testing.T) {
		require.NoError(t, ValidateDBName("db"))
		require.NoError(t, ValidateCollectionName("col"))
		require.NoError(t, ValidateFullNamespace("db.col"))
	})
}
