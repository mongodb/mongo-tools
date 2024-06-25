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
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

func TestNewMongoWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When building write concern object", t, func() {
		Convey("and given a write concern string value, and a boolean indicating if the "+
			"write concern is to be used on a replica set, on calling NewMongoWriteConcern...", func() {
			Convey("no error should be returned if the write concern is valid", func() {
				writeConcern, err := NewMongoWriteConcern(`{w:34}`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, 34)

				writeConcern, err = NewMongoWriteConcern(`{w:"majority"}`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, majString)

				writeConcern, err = NewMongoWriteConcern(`majority`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, majString)

				writeConcern, err = NewMongoWriteConcern(`tagset`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, "tagset")
			})
			Convey("with a w value of 0, without j set, an unack'd write concern should be returned", func() {
				writeConcern, err := NewMongoWriteConcern(`{w:0}`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, 0)
			})
			Convey("with a negative w value, an error should be returned", func() {
				writeConcern, err := NewMongoWriteConcern(`{w:-1}`, nil)
				So(writeConcern, ShouldBeNil)
				So(err, ShouldNotBeNil)
				writeConcern, err = NewMongoWriteConcern(`{w:-2}`, nil)
				So(writeConcern, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
			Convey("with a w value of 0, with j set, a non-nil write concern should be returned", func() {
				writeConcern, err := NewMongoWriteConcern(`{w:0, j:true}`, nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, 0)
				So(writeConcern.GetJ(), ShouldBeTrue)
			})
			// Regression test for TOOLS-1741
			Convey("When passing an empty writeConcern and empty URI"+
				"then write concern should default to being majority", func() {
				writeConcern, err := NewMongoWriteConcern("", nil)
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, majString)
			})
		})
		Convey("and given a connection string", func() {
			Convey("with a w value of 0, without j set, an unack'd write concern should be returned", func() {
				writeConcern, err := NewMongoWriteConcern(``, &connstring.ConnString{WNumber: 0, WNumberSet: true})
				So(err, ShouldBeNil)
				So(writeConcern.GetW(), ShouldEqual, 0)
			})
			Convey("with a negative w value, an error should be returned", func() {
				_, err := NewMongoWriteConcern(``, &connstring.ConnString{WNumber: -1, WNumberSet: true})
				So(err, ShouldNotBeNil)
				_, err = NewMongoWriteConcern(``, &connstring.ConnString{WNumber: -2, WNumberSet: true})
				So(err, ShouldNotBeNil)
			})
		})
		Convey("and given both, should prefer commandline", func() {
			writeConcern, err := NewMongoWriteConcern(`{w: 4}`, &connstring.ConnString{WNumber: 0, WNumberSet: true})
			So(err, ShouldBeNil)
			So(writeConcern.GetW(), ShouldEqual, 4)
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
			So(writeConcern.GetW(), ShouldEqual, majString)
		})

		Convey("Int values should be assigned to the 'w' field ", func() {
			cs := &connstring.ConnString{
				WNumber:    4,
				WNumberSet: true,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.GetW(), ShouldEqual, 4)
		})

		Convey("&connstrings with valid j, wtimeout, and w should be "+
			"assigned accordingly", func() {
			expectedW := 3
			expectedWTimeout := 43 * time.Second
			cs := &connstring.ConnString{
				WNumber:    3,
				WNumberSet: true,
				J:          true,
				WTimeout:   time.Second * 43,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.GetW(), ShouldEqual, expectedW)
			So(writeConcern.GetJ(), ShouldBeTrue)
			So(writeConcern.GetWTimeout(), ShouldEqual, expectedWTimeout)
		})

		Convey("Unacknowledge write concern strings should return a corresponding object "+
			"if journaling is not required", func() {
			cs := &connstring.ConnString{
				WNumber:    0,
				WNumberSet: true,
			}
			writeConcern, err := constructWCFromConnString(cs)
			So(err, ShouldBeNil)
			So(writeConcern.GetW(), ShouldEqual, 0)
		})
	})
}
