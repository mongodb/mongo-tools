// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

type PositionalArgumentTestCase struct {
	InputArgs    []string
	ExpectedOpts Options
	ExpectErr    string
	AuthType     string
}

func TestPositionalArgumentParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

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
			InputArgs: []string{
				"mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
			},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
						ConnString: &connstring.ConnString{
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
						ConnString: &connstring.ConnString{
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
			require.Error(t, err)
			assert.EqualError(t, err, tc.ExpectErr)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedOpts.ConnectionString, opts.ConnectionString)
			switch tc.AuthType {
			case "aws":
				assert.Equal(t, tc.ExpectedOpts.Username, opts.Username)
				assert.Equal(t, tc.ExpectedOpts.Password, opts.Password)
				assert.Equal(t, tc.ExpectedOpts.Mechanism, opts.Mechanism)
				assert.Equal(t, tc.ExpectedOpts.AWSSessionToken, opts.AWSSessionToken)
				assert.Equal(t, tc.ExpectedOpts.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"], opts.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"])
			case "kerberos":
				assert.Equal(t, tc.ExpectedOpts.Username, opts.Username)
				assert.Equal(t, tc.ExpectedOpts.Mechanism, opts.Mechanism)
				assert.Equal(t, tc.ExpectedOpts.Source, opts.Source)
				assert.Equal(t, tc.ExpectedOpts.ConnString.AuthMechanismProperties["SERVICE_NAME"], opts.ConnString.AuthMechanismProperties["SERVICE_NAME"])
				assert.Equal(t, tc.ExpectedOpts.ConnString.AuthMechanismProperties["CANONICALIZE_HOST_NAME"], opts.ConnString.AuthMechanismProperties["CANONICALIZE_HOST_NAME"])
				assert.Equal(t, tc.ExpectedOpts.ConnString.AuthMechanismProperties["SERVICE_REALM"], opts.ConnString.AuthMechanismProperties["SERVICE_REALM"])
				assert.Equal(t, tc.ExpectedOpts.Service, opts.Service)
			}
		}
	}
}
