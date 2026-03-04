// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// validateParseOptions is a helper function to call ParseOptions and verify the results.
// args: command line args
// expectSuccess: whether or not the error from ParseOptions should be nil
// ingestWc: the correct value for opts.IngestOptions.WriteConcern
// toolsWc: the correct value for opts.ToolsOptions.WriteConcern.
func validateParseOptions(
	t *testing.T,
	args []string,
	ingestWc string,
	toolsWc *wcwrapper.WriteConcern,
) {
	opts, err := ParseOptions(args, "", "")
	require.NoError(t, err)

	assert.Equal(t, ingestWc, opts.IngestOptions.WriteConcern)
	assert.Equal(t, toolsWc, opts.ToolOptions.WriteConcern)
}

// Regression test for TOOLS-1741.
func TestWriteConcernWithURIParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	t.Run("default majority", func(t *testing.T) {
		validateParseOptions(t, []string{}, "", wcwrapper.Majority())
	})

	t.Run("no writeconcern in URI", func(t *testing.T) {
		validateParseOptions(t, []string{
			"--uri", "mongodb://localhost:27017/test",
		}, "", wcwrapper.Majority())
	})

	t.Run("parse from URI", func(t *testing.T) {
		validateParseOptions(t, []string{
			"--uri", "mongodb://localhost:27017/test?w=2",
		}, "", wcwrapper.Wrap(&writeconcern.WriteConcern{W: 2}))
	})

	t.Run("parse from command line", func(t *testing.T) {
		validateParseOptions(t, []string{
			"--writeConcern", "{w: 2}",
		}, "{w: 2}", wcwrapper.Wrap(&writeconcern.WriteConcern{W: 2}))
	})

	t.Run("command line takes precedence", func(t *testing.T) {
		validateParseOptions(t, []string{
			"--uri", "mongodb://localhost:27017/test?w=2",
			"--writeConcern", "{w: 3}",
		}, "{w: 3}", wcwrapper.Wrap(&writeconcern.WriteConcern{W: 3}))
	})
}

// Test parsing for the --legacy flag.
func TestLegacyOptionParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

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
			if !tc.expectSuccess {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(
				t,
				tc.expectedLegacy,
				opts.Legacy,
				"legacy mismatch; expected %v, got %v",
				tc.expectedLegacy,
				opts.Legacy,
			)
		})
	}
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
				InputOptions: &InputOptions{},
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
				InputOptions: &InputOptions{},
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
				"provide only one file name and only one MongoDB connection string. " +
				"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
		},
		{
			InputArgs: []string{"foo", "bar", "mongodb://foo"},
			ExpectErr: "error parsing positional arguments: " +
				"provide only one file name and only one MongoDB connection string. " +
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
			require.Error(t, err)
			assert.EqualError(t, err, tc.ExpectErr)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedOpts.File, opts.File)
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
				opts.ConnString.AuthMechanismProperties["SERVICE_NAME"],
				tc.ExpectedOpts.ConnString.AuthMechanismProperties["SERVICE_NAME"],
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
