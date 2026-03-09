// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// var block and functions copied from testutil to avoid import cycle.
var (
	UserAdmin              = "uAdmin"
	UserAdminPassword      = "password"
	CreatedUserNameEnv     = "TOOLS_TESTING_AUTH_USERNAME"
	CreatedUserPasswordEnv = "TOOLS_TESTING_AUTH_PASSWORD"
	PKCS8Password          = "TOOLS_TESTING_PKCS8_PASSWORD"
	kerberosUsername       = "drivers%40LDAPTEST.10GEN.CC"
	kerberosConnection     = "ldaptest.10gen.cc:27017"
)

func DBGetAuthOptions() options.Auth {
	if testtype.HasTestType(testtype.AuthTestType) {
		return options.Auth{
			Username: os.Getenv(CreatedUserNameEnv),
			Password: os.Getenv(CreatedUserPasswordEnv),
			Source:   "admin",
		}
	}

	return options.Auth{}
}

func DBGetSSLOptions() options.SSL {
	if testtype.HasTestType(testtype.SSLTestType) {
		return options.SSL{
			UseSSL:        true,
			SSLCAFile:     "../db/testdata/ca-ia.pem",
			SSLPEMKeyFile: "../db/testdata/test-client.pem",
		}
	}

	return options.SSL{
		UseSSL: false,
	}
}

func DBGetConnString() *options.URI {
	if testtype.HasTestType(testtype.SSLTestType) {
		return &options.URI{
			//ConnectionString: "mongodb://localhost" + DefaultTestPort + "/",
			ConnString: &connstring.ConnString{
				SSLCaFileSet:                   true,
				SSLCaFile:                      "../db/testdata/ca-ia.pem",
				SSLClientCertificateKeyFileSet: true,
				SSLClientCertificateKeyFile:    "../db/testdata/test-client.pem",
			},
		}
	}

	return &options.URI{}
}

func TestNewSessionProvider(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()

	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port: DefaultTestPort,
		},
		URI:  DBGetConnString(),
		SSL:  &ssl,
		Auth: &auth,
	}
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

	_, err = configureClient(*toolOptions)
	require.NoError(t, err)
}

func TestDatabaseNames(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()

	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port: DefaultTestPort,
		},
		URI:  DBGetConnString(),
		SSL:  &ssl,
		Auth: &auth,
	}
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

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()

	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port: DefaultTestPort,
		},
		URI:  DBGetConnString(),
		SSL:  &ssl,
		Auth: &auth,
	}
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

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()
	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port: DefaultTestPort,
		},
		URI:  DBGetConnString(),
		SSL:  &ssl,
		Auth: &auth,
	}
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

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()

	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port: DefaultTestPort,
			Host: "localhost",
		},
		URI:  DBGetConnString(),
		SSL:  &ssl,
		Auth: &auth,
	}
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)

	version, err := provider.ServerVersionArray()
	require.NoError(t, err)
	assert.True(t, version.GT(Version{}))
}

func TestServerCertificateVerification(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testtype.SkipUnlessTestType(t, testtype.SSLTestType)

	auth := DBGetAuthOptions()
	sslOrigin := DBGetSSLOptions()

	// intermediate certs only
	ssl := sslOrigin
	ssl.SSLCAFile = "../db/testdata/ia.pem"
	opts := options.ToolOptions{
		Connection: &options.Connection{
			Port:    DefaultTestPort,
			Timeout: 10,
		},
		URI:  DBGetConnString(),
		SSL:  &ssl,
		Auth: &auth,
	}

	opts.ConnString.SSLCaFile = "../db/testdata/ia.pem"
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)
	require.NoError(t, provider.client.Ping(t.Context(), nil))

	provider.Close()
}

func TestServerPKCS8Verification(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testtype.SkipUnlessTestType(t, testtype.SSLTestType)

	auth := DBGetAuthOptions()
	ssl := options.SSL{
		UseSSL:    true,
		SSLCAFile: "../db/testdata/ca-ia.pem",
	}

	t.Run("with unencrypted password", func(t *testing.T) {
		ssl.SSLPEMKeyFile = "../db/testdata/test-client-pkcs8-unencrypted.pem"
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Port:    DefaultTestPort,
				Timeout: 10,
			},
			URI:  DBGetConnString(),
			SSL:  &ssl,
			Auth: &auth,
		}
		opts.ConnString.SSLCaFile = "../db/testdata/ca-ia.pem"
		provider, err := NewSessionProvider(opts)
		require.NoError(t, err)
		require.NoError(t, provider.client.Ping(t.Context(), nil))
		provider.Close()
	})

	t.Run("with encrypted password", func(t *testing.T) {
		ssl.SSLPEMKeyFile = "../db/testdata/test-client-pkcs8-encrypted.pem"
		ssl.SSLPEMKeyPassword = os.Getenv(PKCS8Password)
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Port:    DefaultTestPort,
				Timeout: 10,
			},
			URI:  DBGetConnString(),
			SSL:  &ssl,
			Auth: &auth,
		}
		opts.ConnString.SSLCaFile = "../db/testdata/ca-ia.pem"
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

	_, err = configureClient(*toolOptions)
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

	_, err = configureClient(*toolOptions)
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

	// We don't set AZURE_FEDERATED_TOKEN_FILE here
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

	_, err = configureClient(*toolOptions)
	require.Error(t, err)
}
