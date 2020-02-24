// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"testing"

	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/testtype"
	. "github.com/smartystreets/goconvey/convey"
)

// validateParseOptions is a helper function to call ParseOptions and verify the results.
// args: command line args
// expectSuccess: whether or not the error from ParseOptions should be nil
// ingestWc: the correct value for opts.IngestOptions.WriteConcern
// toolsWc: the correct value for opts.ToolsOptions.WriteConcern
func validateParseOptions(args []string, expectSuccess bool, ingestWc string, toolsWc *writeconcern.WriteConcern) func() {
	return func() {
		opts, err := ParseOptions(args, "", "")
		if expectSuccess {
			So(err, ShouldBeNil)
		} else {
			So(err, ShouldNotBeNil)
			return
		}

		So(opts.IngestOptions.WriteConcern, ShouldEqual, ingestWc)
		So(opts.ToolOptions.WriteConcern, ShouldResemble, toolsWc)
	}
}

// Regression test for TOOLS-1741
func TestWriteConcernWithURIParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("With an IngestOptions and ToolsOptions", t, func() {
		Convey("Parsing with no value should set a majority write concern",
			validateParseOptions([]string{}, true, "", writeconcern.New(writeconcern.WMajority())))

		Convey("Parsing with no writeconcern in URI should set a majority write concern",
			validateParseOptions([]string{
				"--uri", "mongodb://localhost:27017/test",
			}, true, "", writeconcern.New(writeconcern.WMajority())))

		Convey("Parsing with writeconcern only in URI should set it correctly",
			validateParseOptions([]string{
				"--uri", "mongodb://localhost:27017/test?w=2",
			}, true, "", writeconcern.New(writeconcern.W(2))))

		Convey("Parsing with writeconcern only in command line should set it correctly",
			validateParseOptions([]string{
				"--writeConcern", "{w: 2}",
			}, true, "{w: 2}", writeconcern.New(writeconcern.W(2))))

		Convey("Parsing with writeconcern in URI and command line should set to command line",
			validateParseOptions([]string{
				"--uri", "mongodb://localhost:27017/test?w=2",
				"--writeConcern", "{w: 3}",
			}, true, "{w: 3}", writeconcern.New(writeconcern.W(3))))
	})
}

// Test parsing for the --legacy flag
func TestLegacyOptionParsing(t *testing.T) {
	testCases := []struct {
		name           string
		legacyOpt      string // If "", --legacy will not be included as an option
		expectSuccess  bool
		expectedLegacy bool
	}{
		{"legacy defaults to false", "", true, false},
		{"legacy can be set", "true", true, true},
	}

	baseOpts := []string{"--host", "localhost:27017", "--db", "db", "--collection", "coll"}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := baseOpts
			if tc.legacyOpt != "" {
				args = append(args, "--legacy", tc.legacyOpt)
			}

			opts, err := ParseOptions(args, "", "")
			success := err == nil
			if success != tc.expectSuccess {
				t.Fatalf("expected err to be nil: %v; error was nil: %v", tc.expectSuccess, success)
			}
			if !tc.expectSuccess {
				return
			}

			if opts.Legacy != tc.expectedLegacy {
				t.Fatalf("legacy mismatch; expected %v, got %v", tc.expectedLegacy, opts.Legacy)
			}
		})
	}
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
					InputOptions: &InputOptions{
						File: "foo",
					},
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
					InputOptions: &InputOptions{},
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
					InputOptions: &InputOptions{
						File: "foo",
					},
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
					InputOptions: &InputOptions{
						File: "foo",
					},
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
					InputOptions: &InputOptions{
						File: "foo",
					},
				},
			},
			{
				InputArgs: []string{"--file=foo", "mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					InputOptions: &InputOptions{
						File: "foo",
					},
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
				InputArgs: []string{"mongodb://foo", "foo", "--file=bar"},
				ExpectErr: "error parsing positional arguments: cannot use both --file and a positional argument to set the input file",
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
				So(opts.File, ShouldEqual, tc.ExpectedOpts.File)
				So(opts.ConnectionString, ShouldEqual, tc.ExpectedOpts.ConnectionString)
			}

		}
	})
}
