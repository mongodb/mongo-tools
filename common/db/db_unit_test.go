// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package db

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureClientMultipleHosts(t *testing.T) {
	enabled := options.EnabledOptions{
		Auth:       false,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}

	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err := toolOptions.ParseArgs(
		[]string{"--uri", "mongodb://localhost:27017,localhost:27018/test"},
	)
	require.NoError(t, err)

	err = configureClientAndDisconnectAtEnd(t, *toolOptions)
	require.NoError(t, err)
}

func TestConfigureClientAKS(t *testing.T) {
	// With Azure environment variables

	enabled := options.EnabledOptions{
		Auth:       true,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}

	t.Setenv("AZURE_APP_CLIENT_ID", "test")
	t.Setenv("AZURE_IDENTITY_CLIENT_ID", "test")
	t.Setenv("AZURE_TENANT_ID", "test")
	t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "test")
	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err := toolOptions.ParseArgs(
		[]string{
			"--uri",
			"mongodb://test.net/?directConnection=true&tls=true&authMechanism=MONGODB-OIDC&authMechanismProperties=ENVIRONMENT:azure",
		},
	)
	require.NoError(t, err)

	err = configureClientAndDisconnectAtEnd(t, *toolOptions)
	require.NoError(t, err)
	assert.Equal(t, "MONGODB-OIDC", toolOptions.Mechanism)
}

func TestMisconfigureClientAKS(t *testing.T) {
	enabled := options.EnabledOptions{
		Auth:       true,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}

	// configureClient uses os.LookupEnv, so an ambient AZURE_FEDERATED_TOKEN_FILE
	// would make this configuration succeed. t.Setenv can't express "unset".
	unsetEnvForTest(t, "AZURE_FEDERATED_TOKEN_FILE")
	t.Setenv("AZURE_APP_CLIENT_ID", "test")
	t.Setenv("AZURE_IDENTITY_CLIENT_ID", "test")
	t.Setenv("AZURE_TENANT_ID", "test")
	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err := toolOptions.ParseArgs(
		[]string{
			"--uri",
			"mongodb://test.net/?directConnection=true&tls=true&authMechanism=MONGODB-OIDC&authMechanismProperties=ENVIRONMENT:azure",
		},
	)
	require.NoError(t, err)

	err = configureClientAndDisconnectAtEnd(t, *toolOptions)
	require.Error(t, err)
}

func unsetEnvForTest(t *testing.T, name string) {
	orig, wasSet := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name), "unset %s", name)

	if wasSet {
		t.Cleanup(func() {
			require.NoError(t, os.Setenv(name, orig), "restore %s", name)
		})
	}
}

func TestConfigureClientForSRV(t *testing.T) {
	enabled := options.EnabledOptions{
		Auth:       true,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}

	// AuthSource without a username is invalid, we want to check the URI does not get
	// validated as part of client configuration
	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err := toolOptions.ParseArgs(
		[]string{"--uri", "mongodb://foo/?authSource=admin", "--username", "bar"},
	)
	require.NoError(t, err)

	err = configureClientAndDisconnectAtEnd(t, *toolOptions)
	require.NoError(t, err)
}
