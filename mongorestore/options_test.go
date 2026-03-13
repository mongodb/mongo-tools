// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

func TestWriteConcernOptionParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	t.Run("default to majority", func(t *testing.T) {
		opts, err := ParseOptions([]string{}, "", "")
		require.NoError(t, err)

		assert.Equal(t, "", opts.OutputOptions.WriteConcern)
		assert.Equal(t, wcwrapper.Majority(), opts.ToolOptions.WriteConcern)
	})

	t.Run("no write concern in URI", func(t *testing.T) {
		args := []string{
			"--uri", "mongodb://localhost:27017/test",
		}
		opts, err := ParseOptions(args, "", "")
		require.NoError(t, err)

		assert.Equal(t, wcwrapper.Majority(), opts.ToolOptions.WriteConcern)
	},
	)

	t.Run("parse from URI", func(t *testing.T) {
		args := []string{
			"--uri", "mongodb://localhost:27017/test?w=2",
		}
		opts, err := ParseOptions(args, "", "")
		require.NoError(t, err)

		assert.Equal(t, "", opts.OutputOptions.WriteConcern)
		assert.Equal(
			t,
			wcwrapper.Wrap(&writeconcern.WriteConcern{W: 2}),
			opts.ToolOptions.WriteConcern,
		)
	})

	t.Run("parse from command line", func(t *testing.T) {
		args := []string{
			"--writeConcern", "{w: 2, j: true}",
		}
		opts, err := ParseOptions(args, "", "")
		require.NoError(t, err)

		assert.Equal(
			t,
			wcwrapper.Wrap(&writeconcern.WriteConcern{W: 2, Journal: lo.ToPtr(true)}),
			opts.ToolOptions.WriteConcern,
		)
	})
}

func TestURIParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	args := []string{
		"--uri", "mongodb://localhost:27017/test",
	}
	opts, err := ParseOptions(args, "", "")

	require.NoError(t, err)
	assert.Equal(t, "test", opts.DB)
}

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
				TargetDirectory: "",
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
				TargetDirectory: "",
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
			require.Error(t, err)
			assert.EqualError(t, err, tc.ExpectErr)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedOpts.TargetDirectory, opts.TargetDirectory)
			assert.Equal(t, tc.ExpectedOpts.ConnectionString, opts.ConnectionString)
		}
		switch tc.AuthType {
		case "aws":
			assert.Equal(t, tc.ExpectedOpts.Username, opts.Username)
			assert.Equal(t, tc.ExpectedOpts.Password, opts.Password)
			assert.Equal(t, tc.ExpectedOpts.Mechanism, opts.Mechanism)
			assert.Equal(t, tc.ExpectedOpts.AWSSessionToken, opts.AWSSessionToken)
			assert.Equal(
				t,
				tc.ExpectedOpts.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"],
				opts.ConnString.AuthMechanismProperties["AWS_SESSION_TOKEN"],
			)
		case "kerberos":
			assert.Equal(t, tc.ExpectedOpts.Username, opts.Username)
			assert.Equal(t, tc.ExpectedOpts.Mechanism, opts.Mechanism)
			assert.Equal(t, tc.ExpectedOpts.Source, opts.Source)
			assert.Equal(
				t,
				tc.ExpectedOpts.ConnString.AuthMechanismProperties["SERVICE_NAME"],
				opts.ConnString.AuthMechanismProperties["SERVICE_NAME"],
			)
			assert.Equal(
				t,
				tc.ExpectedOpts.ConnString.AuthMechanismProperties["CANONICALIZE_HOST_NAME"],
				opts.ConnString.AuthMechanismProperties["CANONICALIZE_HOST_NAME"],
			)
			assert.Equal(
				t,
				tc.ExpectedOpts.ConnString.AuthMechanismProperties["SERVICE_REALM"],
				opts.ConnString.AuthMechanismProperties["SERVICE_REALM"],
			)
			assert.Equal(t, tc.ExpectedOpts.Service, opts.Service)
		}
	}
}
