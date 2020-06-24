// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"testing"

	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/testtype"
	. "github.com/smartystreets/goconvey/convey"
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
						},
						Auth: &options.Auth{
							Username: "user",
							Password: "pass",
							AWSSessionToken: "token",
							Mechanism: "MONGODB-AWS",
						},
					},
				},
				AuthType: "aws",
			},
			{
				InputArgs: []string{"mongodb://user@localhost/kerberos?authSource=$external&authMechanism=GSSAPI"},
				ExpectedOpts: Options{
					ToolOptions: &options.ToolOptions{
						URI: &options.URI{
							ConnectionString: "mongodb://user@localhost/kerberos?authSource=$external&authMechanism=GSSAPI",
						},
						Auth: &options.Auth{
							Username: "user",
							Source: "$external",
							Mechanism: "GSSAPI",
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
				InputArgs: []string{"foo"},
				ExpectErr: "error parsing positional arguments: " +
					"provide only one MongoDB connection string. " +
					"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
			},
			{
				InputArgs: []string{"foo", "mongodb://foo"},
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
				if tc.AuthType == "aws" {
					So(opts.Auth.Username, ShouldEqual, tc.ExpectedOpts.Auth.Username)
					So(opts.Auth.Password, ShouldEqual, tc.ExpectedOpts.Auth.Password)
					So(opts.Auth.Mechanism, ShouldEqual, tc.ExpectedOpts.Auth.Mechanism)
					So(opts.Auth.AWSSessionToken, ShouldEqual, tc.ExpectedOpts.Auth.AWSSessionToken)
				} else if tc.AuthType == "kerberos" {
					So(opts.Auth.Username, ShouldEqual, tc.ExpectedOpts.Auth.Username)
					So(opts.Auth.Mechanism, ShouldEqual, tc.ExpectedOpts.Auth.Mechanism)
					So(opts.Auth.Source, ShouldEqual, tc.ExpectedOpts.Auth.Source)
				}
			}
		}
	})
}
