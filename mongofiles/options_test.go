// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"github.com/mongodb/mongo-go-driver/mongo/writeconcern"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWriteConcernOptionParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Testing write concern parsing from command line and URI", t, func() {
		Convey("Parsing with neither URI nor command line option should leave write concern empty", func() {
			storageOpts := &StorageOptions{}
			inputOpts := &InputOptions{}
			_, opts, err := ParseOptions([]string{}, storageOpts, inputOpts)

			So(err, ShouldBeNil)
			So(storageOpts.WriteConcern, ShouldEqual, "")

			Convey("and building write concern object, WMode should be majority", func() {
				So(opts.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.WMajority()))
			})
		})

		Convey("Parsing with URI with no write concern specified in it should not error", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test",
			}

			storageOpts := &StorageOptions{}
			inputOpts := &InputOptions{}
			_, opts, err := ParseOptions(args, storageOpts, inputOpts)

			So(err, ShouldBeNil)
			So(opts.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		Convey("Parsing with write concern in both URI and command line should prefer command line", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test?w=majority",
				"--writeConcern", "{w: 2}",
			}

			storageOpts := &StorageOptions{}
			inputOpts := &InputOptions{}
			_, opts, err := ParseOptions(args, storageOpts, inputOpts)

			So(err, ShouldBeNil)
			So(opts.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2)))
		})

		Convey("Parsing with writeconcern only in URI should set it correctly", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test?w=2",
			}

			storageOpts := &StorageOptions{}
			inputOpts := &InputOptions{}
			_, opts, err := ParseOptions(args, storageOpts, inputOpts)

			So(err, ShouldBeNil)
			So(opts.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2)))
		})

		Convey("Parsing with writeconcern only in command line should set it correctly", func() {
			args := []string{
				"--writeConcern", "{w: 2, j: true}",
			}

			storageOpts := &StorageOptions{}
			inputOpts := &InputOptions{}
			_, opts, err := ParseOptions(args, storageOpts, inputOpts)

			So(err, ShouldBeNil)
			So(opts.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2), writeconcern.J(true)))
		})
	})
}
