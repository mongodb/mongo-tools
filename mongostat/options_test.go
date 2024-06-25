// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongostat

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

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
				InputArgs: []string{"2"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://localhost/",
						},
					},
					SleepInterval: 2,
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

					SleepInterval: 1,
				},
			},
			{
				InputArgs: []string{"mongodb://foo", "2"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					SleepInterval: 2,
				},
			},
			{
				InputArgs: []string{"2", "mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					SleepInterval: 2,
				},
			},
			{
				InputArgs: []string{"2", "--uri=mongodb://foo"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://foo",
						},
					},
					SleepInterval: 2,
				},
			},
			{
				InputArgs: []string{
					"mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
				},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
							ConnString: connstring.ConnString{
								AuthMechanismProperties: map[string]string{
									"AWS_SESSION_TOKEN": "token",
								},
							},
						},
						Auth: &options.Auth{
							Username:        "user",
							Password:        "pass",
							AWSSessionToken: "token",
							Mechanism:       "MONGODB-AWS",
						},
					},
					SleepInterval: 1,
				},
				AuthType: "aws",
			},
			{
				InputArgs: []string{
					"mongodb://user@localhost/kerberos?authSource=$external&authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:service,CANONICALIZE_HOST_NAME:host,SERVICE_REALM:realm",
				},
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
					SleepInterval: 1,
				},
				AuthType: "kerberos",
			},
			{
				InputArgs: []string{"mongodb://foo", "mongodb://bar"},
				ExpectErr: "too many URIs found in positional arguments: only one URI can be set as a positional argument",
			},
			{
				InputArgs: []string{"2", "3"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one polling interval in seconds and only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"2", "3", "mongodb://foo"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one polling interval in seconds and only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"mongodb://foo", "--uri=mongodb://bar"},
				ExpectErr: "illegal argument combination: cannot specify a URI in a positional argument and --uri",
			},
			{
				InputArgs: []string{"mongodb://foo", "2", "--uri=mongodb://bar"},
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
				So(opts.SleepInterval, ShouldEqual, tc.ExpectedOpts.SleepInterval)
				So(opts.ConnectionString, ShouldEqual, tc.ExpectedOpts.ConnectionString)
			}
			if tc.AuthType == "aws" {
				So(opts.Auth.Username, ShouldEqual, tc.ExpectedOpts.Auth.Username)
				So(opts.Auth.Password, ShouldEqual, tc.ExpectedOpts.Auth.Password)
				So(opts.Auth.Mechanism, ShouldEqual, tc.ExpectedOpts.Auth.Mechanism)
				So(opts.Auth.AWSSessionToken, ShouldEqual, tc.ExpectedOpts.Auth.AWSSessionToken)
				So(
					opts.URI.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"],
					ShouldEqual,
					tc.ExpectedOpts.URI.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"],
				)
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
