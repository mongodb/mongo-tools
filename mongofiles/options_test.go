// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

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

	t.Run("default majority", func(t *testing.T) {
		opts, err := ParseOptions([]string{}, "", "")
		require.NoError(t, err)

		assert.Equal(t, "", opts.StorageOptions.WriteConcern)
		assert.Equal(t, wcwrapper.Majority(), opts.ToolOptions.WriteConcern)
	})

	t.Run("default majority with URI", func(t *testing.T) {
		args := []string{
			"--uri", "mongodb://localhost:27017/test",
		}
		opts, err := ParseOptions(args, "", "")
		require.NoError(t, err)

		assert.Equal(t, "", opts.StorageOptions.WriteConcern)
		assert.Equal(t, wcwrapper.Majority(), opts.ToolOptions.WriteConcern)
	})

	t.Run("parse from URI", func(t *testing.T) {
		args := []string{
			"--uri", "mongodb://localhost:27017/test?w=2",
		}
		opts, err := ParseOptions(args, "", "")
		require.NoError(t, err)

		assert.Equal(t, "", opts.StorageOptions.WriteConcern)
		assert.Equal(
			t,
			wcwrapper.Wrap(&writeconcern.WriteConcern{W: 2}),
			opts.ToolOptions.WriteConcern,
		)
	})

	t.Run("parse from CLI", func(t *testing.T) {
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

type PositionalArgumentTestCase struct {
	InputArgs    []string
	ExpectedOpts Options
	ExpectedMF   MongoFiles
	ExpectErr    string
	AuthType     string
}

func TestPositionalArgumentParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	positionalArgumentTestCases := []PositionalArgumentTestCase{
		{
			InputArgs: []string{"list", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "list",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "list", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "list",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "list"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "",
				Command:  "list",
			},
		},
		{
			InputArgs: []string{"search", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "search",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "search", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "search",
			},
		},
		{
			InputArgs: []string{"put", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameList: []string{"foo"},
				Command:      "put",
			},
		},
		{
			InputArgs: []string{"put", "foo", "bar", "baz"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameList: []string{"foo", "bar", "baz"},
				Command:      "put",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "put", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameList: []string{"foo"},
				Command:      "put",
			},
		},
		{
			InputArgs: []string{"put_id", "foo", "id"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "put_id",
				Id:       "id",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "put_id", "foo", "id"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "put_id",
				Id:       "id",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "get", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameList: []string{"foo"},
				Command:      "get",
			},
		},
		{
			InputArgs: []string{"get", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameList: []string{"foo"},
				Command:      "get",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "get", "foo", "bar", "baz"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameList: []string{"foo", "bar", "baz"},
				Command:      "get",
			},
		},
		{
			InputArgs: []string{"get_regex", "test_regex(\\d)"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileNameRegex: "test_regex(\\d)",
				Command:       "get_regex",
			},
		},
		{
			InputArgs: []string{"get_id", "id"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				Command: "get_id",
				Id:      "id",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "get_id", "id"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				Command: "get_id",
				Id:      "id",
			},
		},
		{
			InputArgs: []string{"delete", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "delete",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "delete", "foo"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "delete",
			},
		},
		{
			InputArgs: []string{"delete_id", "id"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://localhost/",
					},
				},
			},
			ExpectedMF: MongoFiles{
				Command: "delete_id",
				Id:      "id",
			},
		},
		{
			InputArgs: []string{"mongodb://foo", "delete_id", "id"},
			ExpectedOpts: Options{
				ToolOptions: &options.ToolOptions{
					URI: &options.URI{
						ConnectionString: "mongodb://foo",
					},
				},
			},
			ExpectedMF: MongoFiles{
				Command: "delete_id",
				Id:      "id",
			},
		},
		{
			InputArgs: []string{
				"mongodb://user:pass@localhost/aws?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
				"list",
				"foo",
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
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "list",
			},
			AuthType: "aws",
		},
		{
			InputArgs: []string{
				"mongodb://user@localhost/kerberos?authSource=$external&authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:service,CANONICALIZE_HOST_NAME:host,SERVICE_REALM:realm",
				"list",
				"foo",
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
			ExpectedMF: MongoFiles{
				FileName: "foo",
				Command:  "list",
			},
			AuthType: "kerberos",
		},
		{
			InputArgs: []string{"put_id", "mongodb://foo", "mongodb://bar"},
			ExpectErr: "too many URIs found in positional arguments: only one URI can be set as a positional argument",
		},
		{
			InputArgs: []string{"list", "foo", "bar"},
			ExpectErr: "too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
		},
		{
			InputArgs: []string{"list", "foo", "bar", "mongodb://foo"},
			ExpectErr: "too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
		},
		{
			InputArgs: []string{"get"},
			ExpectErr: "'get' argument missing",
		},
		{
			InputArgs: []string{"get", "mongodb://foo"},
			ExpectErr: "'get' argument missing",
		},
		{
			InputArgs: []string{"foo", "bar"},
			ExpectErr: "'foo' is not a valid command (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
		},
		{
			InputArgs: []string{"list", "mongodb://foo", "--uri=mongodb://bar"},
			ExpectErr: "illegal argument combination: cannot specify a URI in a positional argument and --uri",
		},
	}

	for _, tc := range positionalArgumentTestCases {
		t.Logf("Testing: %s", tc.InputArgs)
		var mf *MongoFiles
		opts, err := ParseOptions(tc.InputArgs, "", "")

		if err == nil {
			mf = &MongoFiles{
				ToolOptions: opts.ToolOptions,
				StorageOptions: &StorageOptions{
					GridFSPrefix: "fs",
				},
			}
			err = mf.ValidateCommand(opts.ParsedArgs)
		}

		if tc.ExpectErr != "" {
			require.Error(t, err)
			assert.EqualError(t, err, tc.ExpectErr)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedMF.FileName, mf.FileName)
			assert.Equal(t, tc.ExpectedMF.FileNameList, mf.FileNameList)
			assert.Equal(t, tc.ExpectedMF.FileNameRegex, mf.FileNameRegex)
			assert.Equal(t, tc.ExpectedMF.Command, mf.Command)
			assert.Equal(t, tc.ExpectedMF.Id, mf.Id)
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

func TestGetRegexWithOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// This depends on (*MongoFiles).StorageOptions
	// It needs to be checked separately from "Testing parsing positional arguments"
	args := []string{
		"get_regex",
		"another_regex[a-zA-Z]",
		"--regexOptions",
		"mx",
	}

	opts, err := ParseOptions(args, "", "")
	require.NoError(t, err)

	mf := &MongoFiles{
		ToolOptions:    opts.ToolOptions,
		StorageOptions: opts.StorageOptions,
	}

	err = mf.ValidateCommand(opts.ParsedArgs)
	require.NoError(t, err)

	assert.Equal(t, args[1], mf.FileNameRegex)
	assert.Equal(t, args[3], mf.StorageOptions.RegexOptions)
}
