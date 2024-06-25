// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

func TestParseOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("TestReadPreferenceParsing", func(t *testing.T) {
		// different sets of arguments to pass to ParseOptions
		secondaryCmdLine := []string{"--readPreference", "secondary"}
		slaveOkCmdLine := []string{"--slaveOk"}
		rpSlaveOkCmdLine := []string{"--slaveOk", "--readPreference", "secondary"}
		secondaryURI := []string{"--uri", "mongodb://localhost:27017/db?readPreference=secondary"}
		cmdLineAndURI := []string{"--uri", "mongodb://localhost:27017/db?readPreference=secondary", "--readPreference", "nearest"}

		testCases := []struct {
			name          string
			args          []string
			expectSuccess bool
			inputRp       string
			toolOptionsRp *readpref.ReadPref
		}{
			{"No values defaults to primary", []string{}, true, "", readpref.Primary()},
			{"Only command line", secondaryCmdLine, true, "secondary", readpref.Secondary()},
			{"Only URI", secondaryURI, true, "", readpref.Secondary()},
			{"Both URI and command line defaults to command line", cmdLineAndURI, true, "nearest", readpref.Nearest()},
			{"slaveOk becomes nearest", slaveOkCmdLine, true, "nearest", readpref.Nearest()},
			{"slaveOk and read pref errors", rpSlaveOkCmdLine, false, "", nil},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				opts, err := ParseOptions(tc.args, "", "")

				success := err == nil
				if success != tc.expectSuccess {
					t.Fatalf("expected err to be nil: %v; got error %v", tc.expectSuccess, err)
				}

				if !tc.expectSuccess {
					// Shouldn't compare read preferences if an error was expected
					return
				}

				if opts.InputOptions.ReadPreference != tc.inputRp {
					t.Fatalf("read preference mismatch on InputOptions; expected %v, got %v", tc.inputRp,
						opts.InputOptions.ReadPreference)
				}

				if tc.toolOptionsRp == nil {
					if opts.ToolOptions.ReadPreference != nil {
						t.Fatalf("expected read preference to be nil, got %v", opts.ToolOptions.ReadPreference)
					}
					return
				}

				expectedMode := tc.toolOptionsRp.Mode()
				gotMode := opts.ToolOptions.ReadPreference.Mode()
				if expectedMode != gotMode {
					t.Fatalf("read preference mode mismatch; expected %v, got %v", expectedMode, gotMode)
				}
			})
		}
	})

	t.Run("TestJSONFormat", func(t *testing.T) {
		testCases := []struct {
			name           string
			jsonFormat     JSONFormat // If "", no jsonFormat will be included
			expectSuccess  bool
			expectedFormat JSONFormat
		}{
			{"JSON format defaults to relaxed", "", true, Relaxed},
			{"JSON format can be set", Canonical, true, Canonical},
		}

		baseOpts := []string{"--host", "localhost:27017", "--db", "db", "--collection", "coll"}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				args := baseOpts
				if tc.jsonFormat != "" {
					args = append(args, "--jsonFormat", string(tc.jsonFormat))
				}

				opts, err := ParseOptions(args, "", "")
				success := err == nil
				if success != tc.expectSuccess {
					t.Fatalf("expected err to be nil: %v; error was nil: %v", tc.expectSuccess, success)
				}
				if !tc.expectSuccess {
					return
				}

				if opts.JSONFormat != tc.expectedFormat {
					t.Fatalf("JSON format mismatch; expected %v, got %v", tc.expectedFormat, opts.JSONFormat)
				}
			})
		}
	})
}

type PositionalArgumentTestCase struct {
	InputArgs    []string
	ExpectedOpts Options
	ExpectErr    string
	AuthType     string
}

func TestPositionalArgumentParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Testing parsing positional arguments", t, func() {
		positionalArgumentTestCases := []PositionalArgumentTestCase{
			{
				InputArgs: []string{"mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
				},
			},
			{
				InputArgs: []string{"mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
							ConnString: connstring.ConnString{
								AuthMechanismProperties: map[string]string{"AWS_SESSION_TOKEN": "token"},
							},
						},
						Auth: &options.Auth{
							Username:        "user",
							Password:        "pass",
							AWSSessionToken: "token",
							Mechanism:       "MONGODB-AWS",
						},
					},
				},
				AuthType: "aws",
			},
			{
				InputArgs: []string{"mongodb://user@localhost/kerberos?authSource=$external&authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:service,CANONICALIZE_HOST_NAME:host,SERVICE_REALM:realm"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://user@localhost/kerberos?authSource=$external&authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:service,CANONICALIZE_HOST_NAME:host,SERVICE_REALM:realm",
							ConnString: connstring.ConnString{
								AuthMechanismProperties: map[string]string{
									"SERVICE_NAME":           "service",
									"CANONICALIZE_HOST_NAME": "host",
									"SERVICE_REALM":          "realm",
								},
							},
						},
						Auth: &options.Auth{
							Username:  "user",
							Source:    "$external",
							Mechanism: "GSSAPI",
						},
						Kerberos: &options.Kerberos{
							Service: "service",
						},
					},
				},
				AuthType: "kerberos",
			},
			{
				InputArgs: []string{"mongodb://foo", "mongodb://bar"},
				ExpectErr: "too many URIs found in positional arguments: only one URI can be set as a positional argument",
			},
			{
				InputArgs: []string{"foo", "bar"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"foo", "bar", "mongodb://foo"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"mongodb://foo", "--uri=mongodb://bar"},
				ExpectErr: "illegal argument combination: cannot specify a URI in a positional argument and --uri",
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
				So(opts.ConnectionString, ShouldEqual, tc.ExpectedOpts.ConnectionString)
			}
			if tc.AuthType == "aws" {
				So(opts.Auth.Username, ShouldEqual, tc.ExpectedOpts.Auth.Username)
				So(opts.Auth.Password, ShouldEqual, tc.ExpectedOpts.Auth.Password)
				So(opts.Auth.Mechanism, ShouldEqual, tc.ExpectedOpts.Auth.Mechanism)
				So(opts.Auth.AWSSessionToken, ShouldEqual, tc.ExpectedOpts.Auth.AWSSessionToken)
				So(opts.URI.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"], ShouldEqual, tc.ExpectedOpts.URI.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"])
			} else if tc.AuthType == "kerberos" {
				So(opts.Auth.Username, ShouldEqual, tc.ExpectedOpts.Auth.Username)
				So(opts.Auth.Mechanism, ShouldEqual, tc.ExpectedOpts.Auth.Mechanism)
				So(opts.Auth.Source, ShouldEqual, tc.ExpectedOpts.Auth.Source)
				So(opts.URI.ConnString.AuthMechanismProperties["SERVICE_NAME"], ShouldEqual, tc.ExpectedOpts.URI.ConnString.AuthMechanismProperties["SERVICE_NAME"])
				So(opts.URI.ConnString.AuthMechanismProperties["CANONICALIZE_HOST_NAME"], ShouldEqual, tc.ExpectedOpts.URI.ConnString.AuthMechanismProperties["CANONICALIZE_HOST_NAME"])
				So(opts.URI.ConnString.AuthMechanismProperties["SERVICE_REALM"], ShouldEqual, tc.ExpectedOpts.URI.ConnString.AuthMechanismProperties["SERVICE_REALM"])
				So(opts.Kerberos.Service, ShouldEqual, tc.ExpectedOpts.Kerberos.Service)
			}
		}
	})
}
