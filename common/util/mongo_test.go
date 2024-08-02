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
	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitHostArg(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When extracting the replica set and hosts from a connection"+
		" host arg", t, func() {

		Convey("an empty host arg should lead to an empty replica set name"+
			" and hosts slice", func() {
			hosts, setName := SplitHostArg("")
			So(hosts, ShouldResemble, []string{""})
			So(setName, ShouldEqual, "")
		})

		Convey("a trailing slsah should lead to a replica set name"+
			" and empty hosts", func() {
			hosts, setName := SplitHostArg("foo/")
			So(hosts, ShouldResemble, []string{""})
			So(setName, ShouldEqual, "foo")
		})

		Convey("a host arg not specifying a replica set name should lead to"+
			" an empty replica set name", func() {
			hosts, setName := SplitHostArg("host1,host2")
			So(hosts, ShouldResemble, []string{"host1", "host2"})
			So(setName, ShouldEqual, "")
		})

		Convey("a host arg specifying a replica set name should lead to that name"+
			" being returned", func() {
			hosts, setName := SplitHostArg("foo/host1,host2")
			So(hosts, ShouldResemble, []string{"host1", "host2"})
			So(setName, ShouldEqual, "foo")
		})

		Convey("a host arg shouldn't have host order altered"+
			" being returned", func() {
			hosts, setName := SplitHostArg("foo/host2,host1")
			So(hosts, ShouldResemble, []string{"host2", "host1"})
			So(setName, ShouldEqual, "foo")
		})

		Convey("a host arg with host/port pairs should preserve the pairs"+
			" in the return", func() {
			hosts, setName := SplitHostArg("foo/host1:27017,host2:27017")
			So(hosts, ShouldResemble, []string{"host1:27017", "host2:27017"})
			So(setName, ShouldEqual, "foo")
		})

	})

}

func TestCreateConnectionAddrs(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When creating the slice of connection addresses", t, func() {

		Convey("if no port is specified, the addresses should all appear"+
			" unmodified in the result", func() {

			addrs := CreateConnectionAddrs("host1,host2", "")
			So(addrs, ShouldResemble, []string{"host1", "host2"})

		})

		Convey("if a port is specified, it should be appended to each host"+
			" from the host connection string", func() {

			addrs := CreateConnectionAddrs("host1,host2", "20000")
			So(addrs, ShouldResemble, []string{"host1:20000", "host2:20000"})

		})

	})

}

func TestBuildURI(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("Generating URIs from host and port arguments", t, func() {
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
			label := fmt.Sprintf("'%s', '%s' should give '%s'", c.h, c.p, c.u)
			Convey(label, func() {
				got := BuildURI(c.h, c.p)
				So(got, ShouldEqual, c.u)
			})
		}
	})
}

func TestInvalidNames(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("Checking some invalid collection names, ", t, func() {
		Convey("test.col$ is invalid", func() {
			So(ValidateDBName("test"), ShouldBeNil)
			So(ValidateCollectionName("col$"), ShouldNotBeNil)
			So(ValidateFullNamespace("test.col$"), ShouldNotBeNil)
		})
		Convey("db/aaa.col is invalid", func() {
			So(ValidateDBName("db/aaa"), ShouldNotBeNil)
			So(ValidateCollectionName("col"), ShouldBeNil)
			So(ValidateFullNamespace("db/aaa.col"), ShouldNotBeNil)
		})
		Convey("db. is invalid", func() {
			So(ValidateDBName("db"), ShouldBeNil)
			So(ValidateCollectionName(""), ShouldNotBeNil)
			So(ValidateFullNamespace("db."), ShouldNotBeNil)
		})
		Convey("db space.col is invalid", func() {
			So(ValidateDBName("db space"), ShouldNotBeNil)
			So(ValidateCollectionName("col"), ShouldBeNil)
			So(ValidateFullNamespace("db space.col"), ShouldNotBeNil)
		})
		Convey("db x$x is invalid", func() {
			So(ValidateDBName("x$x"), ShouldNotBeNil)
			So(ValidateFullNamespace("x$x.y"), ShouldNotBeNil)
		})
		Convey("[null].[null] is invalid", func() {
			So(ValidateDBName("\x00"), ShouldNotBeNil)
			So(ValidateCollectionName("\x00"), ShouldNotBeNil)
			So(ValidateFullNamespace("\x00.\x00"), ShouldNotBeNil)
		})
		Convey("[empty] is invalid", func() {
			So(ValidateFullNamespace(""), ShouldNotBeNil)
		})
		Convey("db.col is valid", func() {
			So(ValidateDBName("db"), ShouldBeNil)
			So(ValidateCollectionName("col"), ShouldBeNil)
			So(ValidateFullNamespace("db.col"), ShouldBeNil)
		})

	})

}
