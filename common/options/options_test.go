// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package options

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
	"go.mongodb.org/mongo-driver/x/mongo/driver/dns"
)

const (
	ShouldSucceed = iota
	ShouldFail
)

func TestLogUnsupportedOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	var buffer bytes.Buffer

	log.SetWriter(&buffer)
	defer log.SetWriter(os.Stderr)

	t.Run("no warning should be logged if there are no unsupported options", func(t *testing.T) {
		args := []string{"mongodb://mongodb.test.com:27017"}

		enabled := EnabledOptions{true, true, true, true}
		opts := New("", "", "", "", true, enabled)

		_, err := opts.ParseArgs(args)
		require.NoError(t, err)

		opts.LogUnsupportedOptions()

		result := buffer.String()
		require.Empty(t, result)
	})

	t.Run("a warning should be logged if there is an unsupported option", func(t *testing.T) {
		args := []string{"mongodb://mongodb.test.com:27017/?foo=bar"}

		enabled := EnabledOptions{true, true, true, true}
		opts := New("", "", "", "", true, enabled)

		_, err := opts.ParseArgs(args)
		require.NoError(t, err)

		opts.LogUnsupportedOptions()

		result := buffer.String()
		expectedResult := fmt.Sprintf(unknownOptionsWarningFormat, "foo")

		require.Contains(t, result, expectedResult)
	})
}

func TestVerbosityFlag(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	enabled := EnabledOptions{false, false, false, false}
	optPtr := New("", "", "", "", true, enabled)
	require.NotNil(t, optPtr)
	require.NotNil(t, optPtr.parser)

	t.Run("no verbosity flags, Level should be 0", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{})
		require.NoError(t, err)
		require.Equal(t, 0, optPtr.Level())
	})

	t.Run("one short verbosity flag, Level should be 1", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"-v"})
		require.NoError(t, err)
		require.Equal(t, 1, optPtr.Level())
	})

	t.Run("three short verbosity flags (consecutive), Level should be 3", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"-vvv"})
		require.NoError(t, err)
		require.Equal(t, 3, optPtr.Level())
	})

	t.Run("three short verbosity flags (dispersed), Level should be 3", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"-v", "-v", "-v"})
		require.NoError(t, err)
		require.Equal(t, 3, optPtr.Level())
	})

	t.Run("short verbosity flag assigned to 3, Level should be 3", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"-v=3"})
		require.NoError(t, err)
		require.Equal(t, 3, optPtr.Level())
	})

	t.Run("consecutive short flags with assignment, only assignment holds", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"-vv=3"})
		require.NoError(t, err)
		require.Equal(t, 3, optPtr.Level())
	})

	t.Run("one long verbose flag, Level should be 1", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"--verbose"})
		require.NoError(t, err)
		require.Equal(t, 1, optPtr.Level())
	})

	t.Run("three long verbosity flags, Level should be 3", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"--verbose", "--verbose", "--verbose"})
		require.NoError(t, err)
		require.Equal(t, 3, optPtr.Level())
	})

	t.Run("long verbosity flag assigned to 3, Level should be 3", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"--verbose=3"})
		require.NoError(t, err)
		require.Equal(t, 3, optPtr.Level())
	})

	t.Run("mixed assignment and bare flag, total is sum", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"--verbose", "--verbose=3"})
		require.NoError(t, err)
		require.Equal(t, 4, optPtr.Level())
	})

	t.Run("run CallArgParser multiple times, level should be correct", func(t *testing.T) {
		_, err := optPtr.CallArgParser([]string{"--verbose", "--verbose=3"})
		require.NoError(t, err)
		_, err = optPtr.CallArgParser([]string{"--verbose", "--verbose=3"})
		require.NoError(t, err)
		require.Equal(t, 4, optPtr.Level())
	})

}

type uriTester struct {
	Name                     string
	CS                       connstring.ConnString
	OptsIn                   *ToolOptions
	OptsExpected             *ToolOptions
	WithGSSAPI               bool
	ShouldError              bool
	AuthShouldAskForPassword bool
}

func TestParseAndSetOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	FalseValue := false

	enabledURIOnly := EnabledOptions{false, false, false, true}
	testCases := []uriTester{
		{
			Name: "built with ssl",
			CS: connstring.ConnString{
				SSL:    true,
				SSLSet: true,
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL: &SSL{
					UseSSL: true,
				},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: enabledURIOnly,
			},
			ShouldError: false,
		},
		{
			Name: "built with ssl using SRV",
			CS: connstring.ConnString{
				SSL:      true,
				SSLSet:   true,
				Original: "mongodb+srv://example.com/",
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL: &SSL{
					UseSSL: true,
				},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: enabledURIOnly,
			},
			ShouldError: false,
		},
		{
			Name: "not built with gssapi",
			CS: connstring.ConnString{
				AuthMechanism: "GSSAPI",
			},
			WithGSSAPI:   false,
			OptsIn:       New("", "", "", "", true, enabledURIOnly),
			OptsExpected: New("", "", "", "", true, enabledURIOnly),
			ShouldError:  true,
		},
		{
			Name: "built with gssapi",
			CS: connstring.ConnString{
				AuthMechanism: "GSSAPI",
				AuthMechanismProperties: map[string]string{
					"SERVICE_NAME": "service",
				},
				AuthMechanismPropertiesSet: true,
			},
			WithGSSAPI: true,
			OptsIn:     New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL:        &SSL{},
				Auth:       &Auth{},
				Namespace:  &Namespace{},
				Kerberos: &Kerberos{
					Service: "service",
				},
				enabledOptions: enabledURIOnly,
			},
			ShouldError: false,
		},
		{
			Name: "connection fields set",
			CS: connstring.ConnString{
				ConnectTimeout:    time.Duration(100) * time.Millisecond,
				ConnectTimeoutSet: true,
				SocketTimeout:     time.Duration(200) * time.Millisecond,
				SocketTimeoutSet:  true,
			},
			OptsIn: &ToolOptions{
				General:   &General{},
				Verbosity: &Verbosity{},
				Connection: &Connection{
					Timeout: 3, // The default value
				},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			OptsExpected: &ToolOptions{
				General:   &General{},
				Verbosity: &Verbosity{},
				Connection: &Connection{
					Timeout:       100,
					SocketTimeout: 200,
				},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			ShouldError: false,
		},
		{
			Name: "auth fields set",
			CS: connstring.ConnString{
				AuthMechanism: "MONGODB-X509",
				AuthSource:    "",
				AuthSourceSet: true,
				Username:      "user",
				Password:      "password",
				PasswordSet:   true,
			},
			OptsIn: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Auth: true, URI: true},
			},
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL:        &SSL{},
				Auth: &Auth{
					Username:  "user",
					Password:  "password",
					Source:    "",
					Mechanism: "MONGODB-X509",
				},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			ShouldError: false,
		},
		{
			Name: "aws auth fields set",
			CS: connstring.ConnString{
				AuthMechanism: "MONGODB-AWS",
				AuthSource:    "",
				AuthSourceSet: true,
				PasswordSet:   true,
				AuthMechanismProperties: map[string]string{
					"AWS_SESSION_TOKEN": "token",
				},
				AuthMechanismPropertiesSet: true,
			},
			OptsIn: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Auth: true, URI: true},
			},
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL:        &SSL{},
				Auth: &Auth{
					Source:          "",
					Mechanism:       "MONGODB-AWS",
					AWSSessionToken: "token",
				},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			ShouldError: false,
		},
		{
			Name: "kerberos fields set but AuthMechanismProperties not init in connString",
			CS: connstring.ConnString{
				AuthMechanism:              "GSSAPI",
				AuthSource:                 "",
				AuthSourceSet:              true,
				PasswordSet:                true,
				AuthMechanismPropertiesSet: false,
			},
			WithGSSAPI: true,
			OptsIn: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL:        &SSL{},
				Auth:       &Auth{},
				Namespace:  &Namespace{},
				Kerberos: &Kerberos{
					Service: "service",
				},
				enabledOptions: EnabledOptions{Auth: true, URI: true},
			},
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL:        &SSL{},
				Auth: &Auth{
					Source:    "",
					Mechanism: "GSSAPI",
				},
				Namespace: &Namespace{},
				Kerberos: &Kerberos{
					Service: "service",
				},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			ShouldError: false,
		},
		{
			Name: "should ask for user password",
			CS: connstring.ConnString{
				AuthMechanism: "MONGODB-X509",
				AuthSource:    "",
				Username:      "user",
			},
			OptsIn: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Auth: true, URI: true},
			},
			OptsExpected: &ToolOptions{
				General:    &General{},
				Verbosity:  &Verbosity{},
				Connection: &Connection{},
				URI:        &URI{},
				SSL:        &SSL{},
				Auth: &Auth{
					Username:  "user",
					Source:    "",
					Mechanism: "MONGODB-X509",
				},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			ShouldError:              false,
			AuthShouldAskForPassword: true,
		},
		{
			Name: "single connect sets 'Direct'",
			CS: connstring.ConnString{
				Connect: connstring.SingleConnect,
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Direct:         true,
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{URI: true},
			},
			ShouldError: false,
		},
		{
			Name: "direct connection sets 'Direct'",
			CS: connstring.ConnString{
				DirectConnection: true,
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Direct:         true,
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{URI: true},
			},
			ShouldError: false,
		},
		{
			Name: "ReplSetName is set when CS contains it",
			CS: connstring.ConnString{
				ReplicaSet: "replset",
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{URI: true},
				ReplicaSetName: "replset",
			},
			ShouldError: false,
		},
		{
			Name: "RetryWrites is set when CS contains it",
			CS: connstring.ConnString{
				RetryWritesSet: true,
				RetryWrites:    false,
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{URI: true},
				RetryWrites:    &FalseValue,
			},
			ShouldError: false,
		},
		{
			Name: "Direct is false when loadbalanced == true",
			CS: connstring.ConnString{
				LoadBalanced:    true,
				LoadBalancedSet: true,
			},
			OptsIn: New("", "", "", "", true, enabledURIOnly),
			OptsExpected: &ToolOptions{
				General:        &General{},
				Verbosity:      &Verbosity{},
				Connection:     &Connection{},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{URI: true},
				ReplicaSetName: "",
				Direct:         false,
			},
			ShouldError: false,
		},
		{
			Name: "Don't fail when uri and options set",
			CS: connstring.ConnString{
				Hosts: []string{"host"},
			},
			OptsIn: &ToolOptions{
				General:   &General{},
				Verbosity: &Verbosity{},
				Connection: &Connection{
					Host: "host",
				},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			OptsExpected: &ToolOptions{
				General:   &General{},
				Verbosity: &Verbosity{},
				Connection: &Connection{
					Host: "host",
				},
				URI:            &URI{},
				SSL:            &SSL{},
				Auth:           &Auth{},
				Namespace:      &Namespace{},
				Kerberos:       &Kerberos{},
				enabledOptions: EnabledOptions{Connection: true, URI: true},
			},
			ShouldError: false,
		},
	}

	for _, testCase := range testCases {
		t.Log("Test Case:", testCase.Name)

		testCase.OptsIn.URI.ConnectionString = "mongodb://dummy"
		testCase.OptsExpected.URI.ConnectionString = "mongodb://dummy"

		BuiltWithGSSAPI = testCase.WithGSSAPI
		defer func() {
			BuiltWithGSSAPI = true
		}()

		testCase.OptsIn.URI.ConnString = testCase.CS

		err := testCase.OptsIn.setOptionsFromURI(testCase.CS)

		if testCase.ShouldError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}

		require.Equal(
			t,
			testCase.OptsExpected.Connection.Timeout,
			testCase.OptsIn.Connection.Timeout,
		)
		require.Equal(
			t,
			testCase.OptsExpected.Connection.SocketTimeout,
			testCase.OptsIn.Connection.SocketTimeout,
		)
		require.Equal(t, testCase.OptsExpected.Username, testCase.OptsIn.Username)
		require.Equal(t, testCase.OptsExpected.Password, testCase.OptsIn.Password)
		require.Equal(t, testCase.OptsExpected.Source, testCase.OptsIn.Source)
		require.Equal(t, testCase.OptsExpected.Auth.Mechanism, testCase.OptsIn.Auth.Mechanism)
		require.Equal(
			t,
			testCase.OptsExpected.Auth.AWSSessionToken,
			testCase.OptsIn.Auth.AWSSessionToken,
		)
		require.Equal(t, testCase.OptsExpected.Direct, testCase.OptsIn.Direct)
		require.Equal(t, testCase.OptsExpected.ReplicaSetName, testCase.OptsIn.ReplicaSetName)
		require.Equal(t, testCase.OptsExpected.SSL.UseSSL, testCase.OptsIn.SSL.UseSSL)
		require.Equal(t, testCase.OptsExpected.Kerberos.Service, testCase.OptsIn.Kerberos.Service)
		require.Equal(
			t,
			testCase.OptsExpected.Kerberos.ServiceHost,
			testCase.OptsIn.Kerberos.ServiceHost,
		)
		require.Equal(t, testCase.OptsExpected.RetryWrites, testCase.OptsIn.RetryWrites)
		require.Equal(
			t,
			testCase.OptsExpected.Auth.ShouldAskForPassword(),
			testCase.OptsIn.Auth.ShouldAskForPassword(),
		)
	}
}

type configTester struct {
	description  string
	yamlBytes    []byte
	expectedOpts *ToolOptions
	outcome      int
}

func runConfigFileTestCases(t *testing.T, testCases []configTester) {
	configFilePath := "./test-config.yaml"
	args := []string{"--config", configFilePath}
	defer os.Remove(configFilePath)

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			if err := os.WriteFile(configFilePath, testCase.yamlBytes, 0644); err != nil {
				require.NoError(t, err)
			}
			opts := New("test", "", "", "", false, EnabledOptions{true, true, true, true})
			err := opts.ParseConfigFile(args)

			if testCase.outcome == ShouldSucceed {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedOpts.Auth.Password, opts.Auth.Password)
				require.Equal(
					t,
					testCase.expectedOpts.URI.ConnectionString,
					opts.URI.ConnectionString,
				)
				require.Equal(
					t,
					testCase.expectedOpts.SSL.SSLPEMKeyPassword,
					opts.SSL.SSLPEMKeyPassword,
				)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func createExpectedOpts(pw string, uri string, ssl string) *ToolOptions {
	opts := New("test", "", "", "", false, EnabledOptions{true, true, true, true})
	opts.Auth.Password = pw
	opts.URI.ConnectionString = uri
	opts.SSL.SSLPEMKeyPassword = ssl
	return opts
}

func TestParseConfigFile(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("should error with no config file specified", func(t *testing.T) {
		opts := New("test", "", "", "", false, EnabledOptions{})

		// --config at beginning of args list
		args := []string{"--config", "--database", "myDB"}
		require.NotNil(t, opts.ParseConfigFile(args))

		// --config at end of args list
		args = []string{"--database", "myDB", "--config"}
		require.NotNil(t, opts.ParseConfigFile(args))

		// --config= at beginning of args list
		args = []string{"--config=", "--database", "myDB"}
		require.NotNil(t, opts.ParseConfigFile(args))

		// --config= at end of args list
		args = []string{"--database", "myDB", "--config="}
		require.NotNil(t, opts.ParseConfigFile(args))
	})

	t.Run("should error with non-existent config file specified", func(t *testing.T) {
		opts := New("test", "", "", "", false, EnabledOptions{})

		// --config with non-existent file
		args := []string{"--config", "DoesNotExist.yaml", "--database", "myDB"}
		require.NotNil(t, opts.ParseConfigFile(args))

		// --config= with non-existent file
		args = []string{"--config=DoesNotExist.yaml", "--database", "myDB"}
		require.NotNil(t, opts.ParseConfigFile(args))
	})

	t.Run("with an existing config file specified", func(t *testing.T) {
		runConfigFileTestCases(t, []configTester{
			{
				"containing nothing (empty file)",
				[]byte(""),
				createExpectedOpts("", "", ""),
				ShouldSucceed,
			},
			{
				"containing only password field",
				[]byte("password: abc123"),
				createExpectedOpts("abc123", "", ""),
				ShouldSucceed,
			},
			{
				"containing only uri field",
				[]byte("uri: abc123"),
				createExpectedOpts("", "abc123", ""),
				ShouldSucceed,
			},
			{
				"containing only sslPEMKeyPassword field",
				[]byte("sslPEMKeyPassword: abc123"),
				createExpectedOpts("", "", "abc123"),
				ShouldSucceed,
			},
			{
				"containing all of password, uri and sslPEMKeyPassword fields",
				[]byte("password: abc123\nuri: def456\nsslPEMKeyPassword: ghi789"),
				createExpectedOpts("abc123", "def456", "ghi789"),
				ShouldSucceed,
			},
			{
				"containing a duplicate field",
				[]byte("password: abc123\npassword: def456"),
				nil,
				ShouldFail,
			},
			{
				"containing an unsupported or misspelled field",
				[]byte("pasword: abc123"),
				nil,
				ShouldFail,
			},
		})
	})

	t.Run("with command line args that override config file values", func(t *testing.T) {
		configFilePath := "./test-config.yaml"
		defer os.Remove(configFilePath)
		if err := os.WriteFile(configFilePath, []byte("password: abc123"), 0644); err != nil {
			require.NoError(t, err)
		}

		t.Run("with --config followed by --password", func(t *testing.T) {
			args := []string{"--config=" + configFilePath, "--password=def456"}
			opts := New("test", "", "", "", false, EnabledOptions{Auth: true})
			_, err := opts.ParseArgs(args)
			require.NoError(t, err)
			require.Equal(t, "def456", opts.Auth.Password)
		})

		t.Run("with --password followed by --config", func(t *testing.T) {
			args := []string{"--password=ghi789", "--config=" + configFilePath}
			opts := New("test", "", "", "", false, EnabledOptions{Auth: true})
			_, err := opts.ParseArgs(args)
			require.NoError(t, err)
			require.Equal(t, "ghi789", opts.Auth.Password)
		})
	})
}

type optionsTester struct {
	options string
	uri     string
	outcome int
}

func createOptionsTestCases(s []string) []optionsTester {
	ret := []optionsTester{
		{fmt.Sprintf("%s %s", s[0], s[2]), "mongodb://user:pass@foo", ShouldSucceed},
		{
			fmt.Sprintf("%s %s", s[0], s[2]),
			fmt.Sprintf("mongodb://user:pass@foo/?%s=%s", s[1], s[2]),
			ShouldSucceed,
		},
		{
			fmt.Sprintf("%s %s", s[0], s[2]),
			fmt.Sprintf("mongodb://user:pass@foo/?%s=%s", s[1], s[3]),
			ShouldFail,
		},
		{"", fmt.Sprintf("mongodb://user:pass@foo/?%s=%s", s[1], s[2]), ShouldSucceed},
	}
	if s[0] == "--ssl" || s[0] == "--sslAllowInvalidCertificates" ||
		s[0] == "--sslAllowInvalidHostnames" ||
		s[0] == "--tlsInsecure" {
		ret[0].options = s[0]
		ret[1].options = s[0]
		ret[2].options = s[0]
	}
	return ret
}

func runOptionsTestCases(t *testing.T, testCases []optionsTester) {
	enabled := EnabledOptions{
		Auth:       true,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}

	for _, c := range testCases {
		argString := fmt.Sprintf("%s --uri %s", c.options, c.uri)
		t.Run(argString, func(t *testing.T) {
			t.Log(argString)
			toolOptions := New("test", "", "", "", true, enabled)
			args := strings.Split(argString, " ")
			_, err := toolOptions.ParseArgs(args)
			if c.outcome == ShouldFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOptionsParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// handwritten test cases
	specialTestCases := []optionsTester{
		// Hosts and Ports
		{"--host foo", "mongodb://foo", ShouldSucceed},
		{"--host foo", "mongodb://bar", ShouldFail},
		{"--port 27018", "mongodb://foo", ShouldSucceed},
		{"--port 27018", "mongodb://foo:27017", ShouldFail},
		{"--port 27018", "mongodb://foo:27019", ShouldFail},
		{"--port 27018", "mongodb://foo:27018", ShouldSucceed},
		{"--host foo:27019 --port 27018", "mongodb://foo", ShouldFail},
		{"--host foo:27018 --port 27018", "mongodb://foo:27018", ShouldSucceed},
		{"--host foo:27019 --port 27018", "mongodb://foo:27018", ShouldFail},

		{"--host foo,bar,baz", "mongodb://foo,bar,baz", ShouldSucceed},
		{"--host foo,bar,baz", "mongodb://baz,bar,foo", ShouldSucceed},
		{
			"--host foo:27018,bar:27019,baz:27020",
			"mongodb://baz:27020,bar:27019,foo:27018",
			ShouldSucceed,
		},
		{
			"--host foo:27018,bar:27019,baz:27020",
			"mongodb://baz:27018,bar:27019,foo:27020",
			ShouldFail,
		},
		{
			"--host foo:27018,bar:27019,baz:27020 --port 27018",
			"mongodb://baz:27018,bar:27019,foo:27020",
			ShouldFail,
		},
		{"--host foo,bar,baz --port 27018", "mongodb://foo,bar,baz", ShouldSucceed},
		{
			"--host foo,bar,baz --port 27018",
			"mongodb://foo:27018,bar:27018,baz:27018",
			ShouldSucceed,
		},
		{"--host foo,bar,baz --port 27018", "mongodb://foo:27018,bar:27019,baz:27020", ShouldFail},

		{"--host repl/foo,bar,baz", "mongodb://foo,bar,baz", ShouldSucceed},
		{"--host repl/foo,bar,baz", "mongodb://foo,bar,baz/?replicaSet=repl", ShouldSucceed},
		{"--host repl/foo,bar,baz", "mongodb://foo,bar,baz/?replicaSet=quux", ShouldFail},

		// Compressors
		{"--compressors snappy", "mongodb://foo/?compressors=snappy", ShouldSucceed},
		{"", "mongodb://foo/?compressors=snappy", ShouldSucceed},
		{"--compressors snappy", "mongodb://foo/", ShouldSucceed},
		{"--compressors snappy", "mongodb://foo/?compressors=zlib", ShouldFail},
		// {"--compressors none", "mongodb://foo/?compressors=snappy", ShouldFail}, // Note: zero value problem
		{"--compressors snappy", "mongodb://foo/?compressors=none", ShouldFail},

		// Auth
		{"--username alice", "mongodb://alice@foo", ShouldSucceed},
		{"--username alice", "mongodb://foo", ShouldSucceed},
		{"--username bob", "mongodb://alice@foo", ShouldFail},
		{"", "mongodb://alice@foo", ShouldSucceed},

		{"--password hunter2", "mongodb://alice@foo", ShouldSucceed},
		{"--password hunter2", "mongodb://alice:hunter2@foo", ShouldSucceed},
		{"--password hunter2", "mongodb://alice:swordfish@foo", ShouldFail},
		{"", "mongodb://alice:hunter2@foo", ShouldSucceed},

		{"--authenticationDatabase db1", "mongodb://user:pass@foo", ShouldSucceed},
		{"--authenticationDatabase db1", "mongodb://user:pass@foo/?authSource=db1", ShouldSucceed},
		{"--authenticationDatabase db1", "mongodb://user:pass@foo/?authSource=db2", ShouldFail},
		{"", "mongodb://user:pass@foo/?authSource=db1", ShouldSucceed},
		{"", "mongodb://a/b:@foo/authSource=db1", ShouldFail},
		{"", "mongodb://user:pass:a@foo/authSource=db1", ShouldFail},
		{"--authenticationDatabase db1", "mongodb://user:pass@foo/db2", ShouldSucceed},
		{
			"--authenticationDatabase db1",
			"mongodb://user:pass@foo/db2?authSource=db1",
			ShouldSucceed,
		},
		{"--authenticationDatabase db1", "mongodb://user:pass@foo/db1?authSource=db2", ShouldFail},

		{
			"--awsSessionToken token",
			"mongodb://user:pass@foo/?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
			ShouldSucceed,
		},
		{
			"",
			"mongodb://user:pass@foo/?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:token",
			ShouldSucceed,
		},
		{
			"--awsSessionToken token",
			"mongodb://user:pass@foo/?authMechanism=MONGODB-AWS&authMechanismProperties=AWS_SESSION_TOKEN:bar",
			ShouldFail,
		},
		{
			"--gssapiServiceName foo, --authenticationMechanism GSSAPI",
			"mongodb://user:pass@foo",
			ShouldSucceed,
		},

		// Namespace
		{"--db db1", "mongodb://foo", ShouldSucceed},
		{"--db db1", "mongodb://foo/db1", ShouldSucceed},
		{"--db db1", "mongodb://foo/db2", ShouldFail},
		{"", "mongodb://foo/db1", ShouldSucceed},
		{"--db db1", "mongodb://user:pass@foo/?authSource=db2", ShouldSucceed},
		{"--db db1", "mongodb://user:pass@foo/db1?authSource=db2", ShouldSucceed},
		{"--db db1", "mongodb://user:pass@foo/db2?authSource=db2", ShouldFail},

		// Kerberos
		{
			"--gssapiServiceName foo",
			"mongodb://user:pass@foo/?authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:foo",
			ShouldSucceed,
		},
		{
			"",
			"mongodb://user:pass@foo/?authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:foo",
			ShouldSucceed,
		},
		{
			"--gssapiServiceName foo",
			"mongodb://user:pass@foo/?authMechanism=GSSAPI&authMechanismProperties=SERVICE_NAME:bar",
			ShouldFail,
		},
		{"--gssapiServiceName foo", "mongodb://user:pass@foo/?authMechanism=GSSAPI", ShouldSucceed},

		// Loadbalanced
		{"", "mongodb://foo,bar/?loadbalanced=true", ShouldFail},
		{"", "mongodb://foo/?loadbalanced=true&replicaSet=foo", ShouldFail},
		{"", "mongodb://foo/?loadbalanced=true&connect=direct", ShouldFail},
	}

	// Each entry is expanded into 4 test cases with createTestCases()
	genericTestCases := [][]string{
		{"--serverSelectionTimeout", "serverSelectionTimeoutMS", "1000", "2000"},
		{"--dialTimeout", "connectTimeoutMS", "1000", "2000"},
		{"--socketTimeout", "socketTimeoutMS", "1000", "2000"},

		{"--authenticationMechanism", "authMechanism", "SCRAM-SHA-1", "GSSAPI"},

		{"--ssl", "ssl", "true", "false"},
		{"--ssl", "tls", "true", "false"},

		{"--sslCAFile", "sslCertificateAuthorityFile", "foo", "bar"},
		{"--sslCAFile", "tlsCAFile", "foo", "bar"},

		{
			"--sslPEMKeyFile",
			"sslClientCertificateKeyFile",
			"../db/testdata/test-client-pkcs8-unencrypted.pem",
			"bar",
		},
		{
			"--sslPEMKeyFile",
			"tlsCertificateKeyFile",
			"../db/testdata/test-client-pkcs8-unencrypted.pem",
			"bar",
		},

		{"--sslPEMKeyPassword", "sslClientCertificateKeyPassword", "foo", "bar"},
		{"--sslPEMKeyPassword", "tlsCertificateKeyFilePassword", "foo", "bar"},

		{"--sslAllowInvalidCertificates", "sslInsecure", "true", "false"},
		{"--sslAllowInvalidCertificates", "tlsInsecure", "true", "false"},

		{"--sslAllowInvalidHostnames", "sslInsecure", "true", "false"},
		{"--sslAllowInvalidHostnames", "tlsInsecure", "true", "false"},

		{"--tlsInsecure", "sslInsecure", "true", "false"},
		{"--tlsInsecure", "tlsInsecure", "true", "false"},
	}

	testCases := []optionsTester{}

	for _, c := range genericTestCases {
		testCases = append(testCases, createOptionsTestCases(c)...)
	}

	testCases = append(testCases, specialTestCases...)

	runOptionsTestCases(t, testCases)
}

func TestParsePositionalArgsAsURI(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	enabled := EnabledOptions{
		Auth:       true,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}
	toolOptions := New("test", "", "", "", true, enabled)

	t.Run("Uri as positional argument", func(t *testing.T) {
		t.Run("schema error is not reported", func(t *testing.T) {
			args := []string{"localhost:27017"}
			_, err := toolOptions.ParseArgs(args)
			require.NoError(t, err)
		})
		t.Run("non-schema errors should be reported", func(t *testing.T) {
			args := []string{"mongodb://a/b@localhost:27017"}
			_, err := toolOptions.ParseArgs(args)
			require.Equal(t, "error parsing uri: unescaped slash in username", err.Error())
		})
	})
}

func TestOptionsParsingForSRV(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	currentDefault := dns.DefaultResolver
	defer func() {
		dns.DefaultResolver = currentDefault
	}()

	mockedHost := "cluster0.2yfpvyw.example.com"
	dns.DefaultResolver = &dns.Resolver{
		LookupSRV: func(srvName, protocol, host string) (string, []*net.SRV, error) {
			if srvName == "mongodb" && protocol == "tcp" && host == mockedHost {
				return "unused",
					[]*net.SRV{
						{
							Target: "ac-1evngnn-shard-00-00.2yfpvyw.example.com.",
							Port:   27017,
						},
						{
							Target: "ac-1evngnn-shard-00-01.2yfpvyw.example.com.",
							Port:   27017,
						},
						{
							Target: "ac-1evngnn-shard-00-02.2yfpvyw.example.com.",
							Port:   27017,
						},
					},
					nil
			}
			return "", nil, fmt.Errorf("unexpected SRV lookup for %q during options parsing", host)
		},
		LookupTXT: func(name string) ([]string, error) {
			if name == mockedHost {
				return []string{"authSource=admin&replicaSet=atlas-fzt5wz-shard-0"}, nil
			}
			return nil, fmt.Errorf("unexpected TXT lookup for %q during options parsing", name)
		},
	}

	atlasURI := fmt.Sprintf("mongodb+srv://%s/", mockedHost)
	cs, err := connstring.Parse(atlasURI)
	if err != nil {
		t.Fatalf("Failed to parse ATLAS_URI (%s): %s", atlasURI, err)
	}

	testCases := []optionsTester{
		{"", atlasURI, ShouldSucceed},
		{"--username foo", atlasURI, ShouldSucceed},
		{"--username foo --password bar", atlasURI, ShouldSucceed},
		{"--username foo --authenticationDatabase admin", atlasURI, ShouldSucceed},
		{"--username foo --authenticationDatabase db1", atlasURI, ShouldFail},
		{"--username foo --ssl", atlasURI, ShouldSucceed},
		{"--username foo --db db1", atlasURI, ShouldSucceed},
		{
			fmt.Sprintf("--username foo --host %s/%s", cs.ReplicaSet, strings.Join(cs.Hosts, ",")),
			atlasURI,
			ShouldSucceed,
		},
		{
			fmt.Sprintf("--username foo --host %s/%s", "wrongReplSet", strings.Join(cs.Hosts, ",")),
			atlasURI,
			ShouldFail,
		},
	}

	runOptionsTestCases(t, testCases)
}

// Regression test for TOOLS-1694 to prevent issue from TOOLS-1115.
func TestHiddenOptionsDefaults(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("With a ToolOptions parsed", func(t *testing.T) {
		enabled := EnabledOptions{Connection: true}
		opts := New("", "", "", "", true, enabled)
		_, err := opts.CallArgParser([]string{})
		require.NoError(t, err)
		t.Run("hidden options should have expected values", func(t *testing.T) {
			require.Equal(t, runtime.NumCPU(), opts.MaxProcs)
			require.Equal(t, 3, opts.Timeout)
			require.Equal(t, 0, opts.SocketTimeout)
		})
	})
}

func TestNamespace_String(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		ns     Namespace
		expect string
	}{
		{Namespace{"foo", "bar"}, "foo.bar"},
		{Namespace{"foo", "bar.baz"}, "foo.bar.baz"},
	}

	for _, c := range cases {
		got := c.ns.String()
		if got != c.expect {
			t.Errorf(
				"invalid string conversion for %#v, got '%s', expected '%s'",
				c.ns,
				got,
				c.expect,
			)
		}
	}

}

func TestDeprecationWarning(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	t.Run("deprecate message", func(t *testing.T) {
		var buffer bytes.Buffer

		log.SetWriter(&buffer)
		defer log.SetWriter(os.Stderr)

		t.Run("Warning for sslAllowInvalidHostnames", func(t *testing.T) {
			enabled := EnabledOptions{Connection: true}
			opts := New("test", "", "", "", true, enabled)
			args := []string{"--ssl", "--sslAllowInvalidHostnames", "mongodb://user:pass@foo/"}
			_, err := opts.ParseArgs(args)
			require.NoError(t, err)
			result := buffer.String()
			require.Contains(t, result, deprecationWarningSSLAllow)
		})

		t.Run("Warning for sslAllowInvalidCertificates", func(t *testing.T) {
			enabled := EnabledOptions{Connection: true}
			opts := New("test", "", "", "", true, enabled)
			args := []string{"--ssl", "--sslAllowInvalidCertificates", "mongodb://user:pass@foo/"}
			_, err := opts.ParseArgs(args)
			require.NoError(t, err)
			result := buffer.String()
			require.Contains(t, result, deprecationWarningSSLAllow)
		})
	})
}

func TestPasswordPrompt(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	pw := "some-password"
	expectPrompt := regexp.MustCompile(`.*password.*:\n`)

	t.Run("no prompt with user unset", func(t *testing.T) {
		stderr, cleanupStderr := mockStderr(t)
		defer cleanupStderr()

		cleanup := mockStdin(t, pw)
		defer cleanup()

		opts := newTestOpts(t)
		err := opts.NormalizeOptionsAndURI()
		require.NoError(t, err)

		prompt, err := os.ReadFile(stderr.Name())
		require.NoError(t, err)
		require.Empty(t, string(prompt))
	})

	t.Run("prompt when user is set and password is not", func(t *testing.T) {
		stderr, cleanupStderr := mockStderr(t)
		defer cleanupStderr()

		cleanup := mockStdin(t, pw)
		defer cleanup()

		opts := newTestOpts(t)
		opts.Auth.Username = "someuser"
		err := opts.NormalizeOptionsAndURI()
		require.NoError(t, err)

		prompt, err := os.ReadFile(stderr.Name())
		require.NoError(t, err)
		require.Regexp(t, expectPrompt, string(prompt))
		require.Equal(t, pw, opts.ConnString.Password)
	})

	t.Run("prompt when host, port, and user are set but uri is not", func(t *testing.T) {
		stderr, cleanupStderr := mockStderr(t)
		defer cleanupStderr()

		cleanup := mockStdin(t, pw)
		defer cleanup()

		opts := newTestOpts(t)
		opts.Auth.Username = "someuser"
		opts.Host = "localhost"
		opts.Port = "12345"
		opts.URI = nil
		err := opts.NormalizeOptionsAndURI()
		require.NoError(t, err)

		prompt, err := os.ReadFile(stderr.Name())
		require.NoError(t, err)
		require.Regexp(t, expectPrompt, string(prompt))
		require.Equal(t, pw, opts.ConnString.Password)
	})

	t.Run("prompt when user is set and password is not and mechanism is PLAIN", func(t *testing.T) {
		stderr, cleanupStderr := mockStderr(t)
		defer cleanupStderr()

		cleanup := mockStdin(t, pw)
		defer cleanup()

		opts := newTestOpts(t)
		opts.Auth.Username = "someuser"
		opts.Auth.Mechanism = "PLAIN"
		opts.SSL.UseSSL = true
		err := opts.NormalizeOptionsAndURI()
		require.NoError(t, err)

		prompt, err := os.ReadFile(stderr.Name())
		require.NoError(t, err)
		require.Regexp(t, expectPrompt, string(prompt))
		require.Equal(t, pw, opts.ConnString.Password)
	})
}

func newTestOpts(t *testing.T) *ToolOptions {
	enabled := EnabledOptions{Auth: true, Connection: true, URI: true}
	opts := New("test", "", "", "", true, enabled)

	var err error
	opts.URI, err = NewURI("mongodb://localhost:12345")
	require.NoError(t, err)

	return opts
}

func mockStderr(t *testing.T) (*os.File, func()) {
	file, err := os.CreateTemp("", "mongo-tools-mock-stderr")
	require.NoError(t, err)

	oldStderr := os.Stderr
	os.Stderr = file

	return file, func() {
		os.Stderr = oldStderr
		file.Close()
		rmErr := os.Remove(file.Name())
		require.NoError(t, rmErr)
	}
}

func mockStdin(t *testing.T, content string) func() {
	file, err := os.CreateTemp("", "mongo-tools-mock-stdin")
	require.NoError(t, err)

	_, err = file.WriteString(content)
	require.NoError(t, err)

	err = file.Close()
	require.NoError(t, err)

	oldStdin := os.Stdin

	file, err = os.Open(file.Name())
	require.NoError(t, err)
	os.Stdin = file

	return func() {
		os.Stdin = oldStdin
		file.Close()
		rmErr := os.Remove(file.Name())
		require.NoError(t, rmErr)
	}
}
