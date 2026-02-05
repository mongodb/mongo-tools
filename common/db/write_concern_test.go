// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

func TestNewMongoWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When building write concern object", t, func() {
		Convey("and given a write concern string value, and a boolean indicating if the "+
			"write concern is to be used on a replica set, on calling NewMongoWriteConcern...", func() {
			Convey("no error should be returned if the write concern is valid", func() {
				writeConcern, err := NewMongoWriteConcern(`{w:34}`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.W, ShouldEqual, 34)

				writeConcern, err = NewMongoWriteConcern(`{w:"majority"}`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.W, ShouldEqual, majString)

				writeConcern, err = NewMongoWriteConcern(`majority`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.W, ShouldEqual, majString)

				writeConcern, err = NewMongoWriteConcern(`tagset`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.W, ShouldEqual, "tagset")
			})
			Convey(
				"with a w value of 0, without j set, an unack'd write concern should be returned",
				func() {
					writeConcern, err := NewMongoWriteConcern(`{w:0}`, nil)
					So(err, ShouldBeNil)
					So(writeConcern.W, ShouldEqual, 0)
				},
			)
			Convey("with a negative w value, an error should be returned", func() {
				writeConcern, err := NewMongoWriteConcern(`{w:-1}`, nil)
				So(writeConcern, ShouldBeNil)
				So(err, ShouldNotBeNil)
				writeConcern, err = NewMongoWriteConcern(`{w:-2}`, nil)
				So(writeConcern, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
			Convey(
				"with a w value of 0, with j set, a non-nil write concern should be returned",
				func() {
					writeConcern, err := NewMongoWriteConcern(`{w:0, j:true}`, nil)
					So(err, ShouldBeNil)
					So(writeConcern.W, ShouldEqual, 0)
					So(*writeConcern.Journal, ShouldBeTrue)
				},
			)
			// Regression test for TOOLS-1741
			Convey("When passing an empty writeConcern and empty URI"+
				"then write concern should default to being majority", func() {
				writeConcern, err := NewMongoWriteConcern("", nil)
				So(err, ShouldBeNil)
				So(writeConcern.W, ShouldEqual, majString)
			})
		})
		Convey("and given a connection string", func() {
			Convey(
				"with a w value of 0, without j set, an unack'd write concern should be returned",
				func() {
					writeConcern, err := NewMongoWriteConcern(
						``,
						&connstring.ConnString{WNumber: 0, WNumberSet: true},
					)
					So(err, ShouldBeNil)
					So(writeConcern.W, ShouldEqual, 0)
				},
			)
			Convey("with a negative w value, an error should be returned", func() {
				_, err := NewMongoWriteConcern(
					``,
					&connstring.ConnString{WNumber: -1, WNumberSet: true},
				)
				So(err, ShouldNotBeNil)
				_, err = NewMongoWriteConcern(
					``,
					&connstring.ConnString{WNumber: -2, WNumberSet: true},
				)
				So(err, ShouldNotBeNil)
			})
		})
		Convey("and given both, should prefer commandline", func() {
			writeConcern, err := NewMongoWriteConcern(
				`{w: 4}`,
				&connstring.ConnString{WNumber: 0, WNumberSet: true},
			)
			So(err, ShouldBeNil)
			So(writeConcern.W, ShouldEqual, 4)
		})
	})
}

func TestConstructWCFromConnString(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("Given a parsed &connstring, on calling constructWCFromConnString...", t, func() {
		Convey("non integer values should set the correct boolean "+
			"field", func() {
			writeConcernString := "majority"
			cs := &connstring.ConnString{
				WString: writeConcernString,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.W, ShouldEqual, majString)
		})

		Convey("Int values should be assigned to the 'w' field ", func() {
			cs := &connstring.ConnString{
				WNumber:    4,
				WNumberSet: true,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.W, ShouldEqual, 4)
		})

		Convey("&connstrings with valid j and w should be assigned accordingly", func() {
			// Note: this used to test WTImeout as well, but the upgrade to Go driver v2 removed wtimeout
			// support from connstring parsing, so we can't/don't do it here any more.
			expectedW := 3
			cs := &connstring.ConnString{
				WNumber:    3,
				WNumberSet: true,
				J:          true,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.W, ShouldEqual, expectedW)
			So(*writeConcern.Journal, ShouldBeTrue)
		})

		Convey("Unacknowledge write concern strings should return a corresponding object "+
			"if journaling is not required", func() {
			cs := &connstring.ConnString{
				WNumber:    0,
				WNumberSet: true,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.W, ShouldEqual, 0)
		})
	})
}
