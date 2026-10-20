// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package db

import (
	"net"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/db/dsctest"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

var PKCS8Password = "TOOLS_TESTING_PKCS8_PASSWORD"

func TestNewSessionProvider(t *testing.T) {
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

func TestIsDisaggregatedStorage(t *testing.T) {
	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err, "creating a session provider")
	defer provider.Close()

	// Against a DSC cluster the detector legitimately reports true, which is not what this test
	// asserts, so skip there.
	client, err := provider.GetSession()
	require.NoError(t, err, "getting a session")
	dsctest.SkipForDisaggregatedStorage(t, client, "the test asserts the non-DSC result")

	isDSC, err := provider.IsDisaggregatedStorage()
	require.NoError(t, err, "checking for disaggregated storage")
	assert.False(t, isDSC, "a normal server reports no disaggregated storage")
}

func TestDatabaseNames(t *testing.T) {
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
	opts := testopts.MustGetToolOptions(t)
	provider, err := NewSessionProvider(opts)
	require.NoError(t, err)

	version, err := provider.ServerVersionArray()
	require.NoError(t, err)
	assert.True(t, version.GT(Version{}))
}

// The certificate tests below build ToolOptions by hand instead of going through `GetToolOptions`,
// so they have to resolve the runner-allocated address from `TOOLS_TESTING_MONGOD` themselves.
func hostAndPortFromEnvVar(t *testing.T) (string, string) {
	t.Helper()

	uri := os.Getenv(testopts.URIEnvVar)
	if uri == "" {
		return "localhost", testopts.DefaultTestPort
	}

	cs, err := connstring.ParseAndValidate(uri)
	require.NoError(t, err, "TOOLS_TESTING_MONGOD is a valid connection string")
	require.NotEmpty(t, cs.Hosts, "TOOLS_TESTING_MONGOD names at least one host")

	host, port, err := net.SplitHostPort(cs.Hosts[0])
	require.NoError(t, err, "TOOLS_TESTING_MONGOD host names a port")

	return host, port
}

func TestServerCertificateVerification(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.SSLTestType)

	auth := testopts.GetAuthOptions()
	sslOrigin := testopts.SSLOptionsWithCert("test-client.pem")

	// intermediate certs only
	ssl := sslOrigin
	ssl.SSLCAFile = testopts.TestDataPath("ia.pem")
	host, port := hostAndPortFromEnvVar(t)
	opts := options.ToolOptions{
		Connection: &options.Connection{
			Host:    host,
			Port:    port,
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
	testtype.SkipUnlessTestType(t, testtype.SSLTestType)

	auth := testopts.GetAuthOptions()
	ssl := options.SSL{
		UseSSL:    true,
		SSLCAFile: testopts.TestDataPath("ca-ia.pem"),
	}

	t.Run("with unencrypted password", func(t *testing.T) {
		ssl.SSLPEMKeyFile = testopts.TestDataPath("test-client-pkcs8-unencrypted.pem")
		host, port := hostAndPortFromEnvVar(t)
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Host:    host,
				Port:    port,
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
		host, port := hostAndPortFromEnvVar(t)
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Host:    host,
				Port:    port,
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
