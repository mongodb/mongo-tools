// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	PKCS8Password      = "TOOLS_TESTING_PKCS8_PASSWORD"
	kerberosUsername   = "drivers%40LDAPTEST.10GEN.CC"
	kerberosConnection = "ldaptest.10gen.cc:27017"
)

func TestNewSessionProvider(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)

	require.NoError(
		t,
		provider.client.Ping(t.Context(), nil),
		"master session successfully initialized",
	)

	provider.Close()
}

func TestConfigureClientForSRV(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

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

func TestDatabaseNames(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)

	err = provider.DropDatabase("exists")
	require.NoError(t, err)
	err = provider.CreateCollection("exists", "collection")
	require.NoError(t, err)
	err = provider.DropDatabase("missingDB")
	require.NoError(t, err)

	names, err := provider.DatabaseNames()
	require.NoError(t, err)
	assert.NotEmpty(t, names)

	m := make(map[string]bool)
	for _, v := range names {
		m[v] = true
	}

	assert.True(t, m["exists"])
	assert.False(t, m["missingDB"])
}

func TestFindOne(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)

	err = provider.DropDatabase("exists")
	require.NoError(t, err)
	err = provider.CreateCollection("exists", "collection")
	require.NoError(t, err)
	client, err := provider.GetSession()
	require.NoError(t, err)
	coll := client.Database("exists").Collection("collection")
	_, err = coll.InsertOne(t.Context(), bson.D{})
	require.NoError(t, err)

	res := bson.D{}
	err = provider.FindOne("exists", "collection", 0, nil, nil, &res, 0)
	require.NoError(t, err)
}

func TestGetIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)
	session, err := provider.GetSession()
	require.NoError(t, err)

	existing := session.Database("exists").Collection("collection")
	missing := session.Database("exists").Collection("missing")
	missingDB := session.Database("missingDB").Collection("missingCollection")

	err = provider.DropDatabase("exists")
	require.NoError(t, err)
	err = provider.CreateCollection("exists", "collection")
	require.NoError(t, err)
	err = provider.DropDatabase("missingDB")
	require.NoError(t, err)

	t.Run("on existing collection", func(t *testing.T) {
		indexesIter, err := GetIndexes(existing)
		require.NoError(t, err)

		require.NotNil(t, indexesIter)
		ctx := t.Context()
		counter := 0
		for indexesIter.Next(ctx) {
			counter++
		}
		assert.NotZero(t, counter)
	})

	t.Run("on missing collection", func(t *testing.T) {
		indexesIter, err := GetIndexes(missing)
		require.NoError(t, err)
		assert.False(t, indexesIter.Next(t.Context()))
	})

	t.Run("on missing database", func(t *testing.T) {
		indexesIter, err := GetIndexes(missingDB)
		require.NoError(t, err)
		assert.False(t, indexesIter.Next(t.Context()))
	})
}

func TestServerVersionArray(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)

	version, err := provider.ServerVersionArray()
	require.NoError(t, err)
	assert.True(t, version.GT(Version{}))
}

func TestServerCertificateVerification(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testtype.SkipUnlessTestType(t, testtype.SSLTestType)

	auth := testopts.GetAuthOptions()
	sslOrigin := testopts.SSLOptionsWithCert("test-client.pem")

	// intermediate certs only
	ssl := sslOrigin
	ssl.SSLCAFile = testopts.TestDataPath("ia.pem")
	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port:    testopts.DefaultTestPort,
			Timeout: 10,
		},
		URI:  testopts.URIWithCert("test-client.pem"),
		SSL:  &ssl,
		Auth: &auth,
	}

	opts.ConnString.SSLCaFile = testopts.TestDataPath("ia.pem")
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)
	require.NoError(t, provider.client.Ping(t.Context(), nil))

	provider.Close()
}

func TestServerPKCS8Verification(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testtype.SkipUnlessTestType(t, testtype.SSLTestType)

	auth := testopts.GetAuthOptions()
	ssl := options.SSL{
		UseSSL:    true,
		SSLCAFile: testopts.TestDataPath("ca-ia.pem"),
	}

	t.Run("with unencrypted password", func(t *testing.T) {
		ssl.SSLPEMKeyFile = testopts.TestDataPath("test-client-pkcs8-unencrypted.pem")
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Port:    testopts.DefaultTestPort,
				Timeout: 10,
			},
			URI:  testopts.URIWithCert("test-client.pem"),
			SSL:  &ssl,
			Auth: &auth,
		}
		opts.ConnString.SSLCaFile = testopts.TestDataPath("ca-ia.pem")
		provider, err := NewSessionProvider(opts)
		require.NoError(t, err)
		require.NoError(t, provider.client.Ping(t.Context(), nil))
		provider.Close()
	})

	t.Run("with encrypted password", func(t *testing.T) {
		ssl.SSLPEMKeyFile = testopts.TestDataPath("test-client-pkcs8-encrypted.pem")
		ssl.SSLPEMKeyPassword = os.Getenv(PKCS8Password)
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Port:    testopts.DefaultTestPort,
				Timeout: 10,
			},
			URI:  testopts.URIWithCert("test-client.pem"),
			SSL:  &ssl,
			Auth: &auth,
		}
		opts.ConnString.SSLCaFile = testopts.TestDataPath("ca-ia.pem")
		provider, err := NewSessionProvider(opts)
		require.NoError(t, err)
		require.NoError(t, provider.client.Ping(t.Context(), nil))
		provider.Close()
	})
}

func TestAuthConnection(t *testing.T) {
	if !testtype.HasTestType(testtype.AWSAuthTestType) &&
		!testtype.HasTestType(testtype.KerberosTestType) {
		t.SkipNow()
	}
	enabled := options.EnabledOptions{URI: true}

	var uri string
	if testtype.HasTestType(testtype.AWSAuthTestType) {
		uriBytes, err := os.ReadFile("../testdata/lib/MONGOD_URI")
		if err != nil {
			panic("Could not read MONGOD_URI file")
		}
		uri = string(uriBytes)
	} else {
		uri = "mongodb://" + kerberosUsername + "@" + kerberosConnection + "/kerberos?authSource=$external&authMechanism=GSSAPI"
	}

	fakeArgs := []string{"--uri=" + uri}
	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err := toolOptions.ParseArgs(fakeArgs)
	if err != nil {
		panic("Could not parse MONGODB_URI file contents")
	}

	_, err = NewSessionProvider(*toolOptions)
	require.NoError(t, err, "connection succeeds")
}

func TestConfigureClientMultipleHosts(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

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
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

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
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

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

// configureClient starts background topology monitoring, so a client that is
// never disconnected leaves goroutines resolving hostnames for the rest of the
// package's test run.
func configureClientAndDisconnectAtEnd(t *testing.T, opts options.ToolOptions) error {
	client, err := configureClient(opts)
	if client != nil {
		t.Cleanup(func() {
			require.NoError(t, client.Disconnect(context.Background()), "disconnect client")
		})
	}

	return err
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
