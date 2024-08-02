// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build failpoints
// +build failpoints

package failpoint

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	c "github.com/smartystreets/goconvey/convey"
)

func TestFailpointParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	c.Convey("With test args", t, func() {
		args := "foo=bar,baz,biz=,=a"
		ParseFailpoints(args)

		c.So(Enabled("foo"), c.ShouldBeTrue)
		c.So(Enabled("baz"), c.ShouldBeTrue)
		c.So(Enabled("biz"), c.ShouldBeTrue)
		c.So(Enabled(""), c.ShouldBeTrue)
		c.So(Enabled("bar"), c.ShouldBeFalse)

		var val string
		var ok bool
		val, ok = Get("foo")
		c.So(val, c.ShouldEqual, "bar")
		c.So(ok, c.ShouldBeTrue)
		val, ok = Get("baz")
		c.So(val, c.ShouldEqual, "")
		c.So(ok, c.ShouldBeTrue)
		val, ok = Get("biz")
		c.So(val, c.ShouldEqual, "")
		c.So(ok, c.ShouldBeTrue)
		val, ok = Get("")
		c.So(val, c.ShouldEqual, "a")
		c.So(ok, c.ShouldBeTrue)
		_, ok = Get("bar")
		c.So(ok, c.ShouldBeFalse)
	})
}
