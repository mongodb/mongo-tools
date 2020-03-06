// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	"testing"
)

func TestWriteConcernOptionParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Testing write concern parsing from command line and URI", t, func() {
		Convey("Parsing with neither URI nor command line option should set a majority write concern", func() {
			opts, err := ParseOptions([]string{}, "", "")

			So(err, ShouldBeNil)
			So(opts.OutputOptions.WriteConcern, ShouldEqual, "")
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		Convey("Parsing with URI with no write concern specified in it should set a majority write concern", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test",
			}
			opts, err := ParseOptions(args, "", "")

			So(err, ShouldBeNil)
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		Convey("Parsing with writeconcern only in URI should set it correctly", func() {
			args := []string{
				"--uri", "mongodb://localhost:27017/test?w=2",
			}
			opts, err := ParseOptions(args, "", "")

			So(err, ShouldBeNil)
			So(opts.OutputOptions.WriteConcern, ShouldEqual, "")
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2)))
		})

		Convey("Parsing with writeconcern only in command line should set it correctly", func() {
			args := []string{
				"--writeConcern", "{w: 2, j: true}",
			}
			opts, err := ParseOptions(args, "", "")

			So(err, ShouldBeNil)
			So(opts.ToolOptions.WriteConcern, ShouldResemble, writeconcern.New(writeconcern.W(2), writeconcern.J(true)))
		})
	})
}

type PositionalArgumentTestCase struct {
	InputArgs    []string
	ExpectedOpts Options
	ExpectErr    string
}

func TestPositionalArgumentParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Testing parsing positional arguments", t, func() {
		positionalArgumentTestCases := []PositionalArgumentTestCase{
			{
				InputArgs: []string{"foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
					TargetDirectory: "foo",
				},
			},
			{
				InputArgs: []string{"mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					TargetDirectory: "",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					TargetDirectory: "foo",
				},
			},
			{
				InputArgs: []string{"foo", "mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					TargetDirectory: "foo",
				},
			},
			{
				InputArgs: []string{"foo", "--uri=mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					TargetDirectory: "foo",
				},
			},
			{
				InputArgs: []string{"--dir=foo", "mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					TargetDirectory: "foo",
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "mongodb://bar"},
				ExpectErr: "too many URIs found in positional arguments: only one URI can be set as a positional argument",
			},
			{
				InputArgs: []string{"foo", "bar"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one polling interval in seconds and only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"foo", "bar", "mongodb://foo"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one polling interval in seconds and only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"mongodb://foo", "--uri=mongodb://bar"},
				ExpectErr: "illegal argument combination: cannot specify a URI in a positional argument and --uri",
			},
			{
				InputArgs: []string{"mongodb://foo", "foo", "--uri=mongodb://bar"},
				ExpectErr: "illegal argument combination: cannot specify a URI in a positional argument and --uri",
			},
			{
				InputArgs: []string{"mongodb://foo", "foo", "--dir=bar"},
				ExpectErr: "error parsing positional arguments: cannot use both --dir and a positional argument to set the target directory",
			},
		}

		for _, tc := range positionalArgumentTestCases {
			t.Logf("Testing: %s", tc.InputArgs)
			opts, err := ParseOptions(tc.InputArgs, "", "")
			if tc.ExpectErr != "" {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, tc.ExpectErr)
			} else {
				So(err, ShouldBeNil)
				So(opts.TargetDirectory, ShouldEqual, tc.ExpectedOpts.TargetDirectory)
				So(opts.ConnectionString, ShouldEqual, tc.ExpectedOpts.ConnectionString)
			}

		}
	})
}
