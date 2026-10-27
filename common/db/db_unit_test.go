// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package db

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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

func TestBuildTLSConfig(t *testing.T) {
	enabled := options.EnabledOptions{
		Auth:       true,
		Connection: true,
		Namespace:  true,
		URI:        true,
	}

	const (
		caFile           = "testdata/ca-ia.pem"
		keyFile          = "testdata/test-client-pkcs8-unencrypted.pem"
		encryptedKeyFile = "testdata/test-client-pkcs8-encrypted.pem"
		keyPassword      = "passwordIsTacoCat"
		certFile         = "testdata/mongodb-test-client.crt"
		privateKeyFile   = "testdata/mongodb-test-client.key"
	)

	testCases := []struct {
		name             string
		uri              string
		args             []string
		expectCAFile     string
		expectCertFile   string
		expectSkipVerify bool
		expectErr        string
	}{
		// The --ssl* flag spelling.
		{
			name: "no CA file leaves the system trust store in charge",
			args: []string{"--ssl"},
		},
		{
			name:         "sslCAFile is loaded into RootCAs",
			args:         []string{"--ssl", "--sslCAFile", caFile},
			expectCAFile: caFile,
		},
		{
			name:             "sslAllowInvalidCertificates skips verification",
			args:             []string{"--ssl", "--sslAllowInvalidCertificates"},
			expectSkipVerify: true,
		},
		{
			name:             "sslAllowInvalidHostnames skips verification",
			args:             []string{"--ssl", "--sslAllowInvalidHostnames"},
			expectSkipVerify: true,
		},
		{
			name:             "tlsInsecure skips verification",
			args:             []string{"--ssl", "--tlsInsecure"},
			expectSkipVerify: true,
		},
		{
			name:           "sslPEMKeyFile loads an unencrypted client key",
			args:           []string{"--ssl", "--sslPEMKeyFile", keyFile},
			expectCertFile: keyFile,
		},
		{
			name: "sslPEMKeyFile loads an encrypted client key with sslPEMKeyPassword",
			args: []string{
				"--ssl",
				"--sslPEMKeyFile", encryptedKeyFile,
				"--sslPEMKeyPassword", keyPassword,
			},
			expectCertFile: encryptedKeyFile,
		},
		{
			name:      "sslFIPSMode is refused",
			args:      []string{"--ssl", "--sslFIPSMode"},
			expectErr: "FIPS mode not supported",
		},
		{
			name:      "sslCRLFile is refused",
			args:      []string{"--ssl", "--sslCRLFile", caFile},
			expectErr: "CRL files are not supported on this platform",
		},
		{
			name:      "a missing CA file is refused",
			args:      []string{"--ssl", "--sslCAFile", "testdata/nonexistent.pem"},
			expectErr: "error configuring client, can't load CA file",
		},
		{
			name: "a wrong client key password is refused",
			args: []string{
				"--ssl",
				"--sslPEMKeyFile", encryptedKeyFile,
				"--sslPEMKeyPassword", "not-the-password",
			},
			expectErr: "error configuring client, can't load client certificate",
		},

		// The tls* URI spelling.
		{
			name: "tls URI option enables TLS with no CA file",
			uri:  "mongodb://localhost:27017/?tls=true",
		},
		{
			name:         "tlsCAFile URI option is loaded into RootCAs",
			uri:          "mongodb://localhost:27017/?tlsCAFile=" + caFile,
			expectCAFile: caFile,
		},
		{
			name:           "tlsCertificateKeyFile URI option loads an unencrypted client key",
			uri:            "mongodb://localhost:27017/?tlsCertificateKeyFile=" + keyFile,
			expectCertFile: keyFile,
		},
		{
			name: "tlsCertificateKeyFilePassword URI option decrypts a client key",
			uri: "mongodb://localhost:27017/?tlsCertificateKeyFile=" + encryptedKeyFile +
				"&tlsCertificateKeyFilePassword=" + keyPassword,
			expectCertFile: encryptedKeyFile,
		},
		{
			name: "tlsCertificateFile and tlsPrivateKeyFile URI options load a client key",
			uri: "mongodb://localhost:27017/?tlsCertificateFile=" + certFile +
				"&tlsPrivateKeyFile=" + privateKeyFile,
			expectCertFile: certFile,
		},
		{
			name:             "tlsInsecure URI option skips verification",
			uri:              "mongodb://localhost:27017/?tls=true&tlsInsecure=true",
			expectSkipVerify: true,
		},

		// The ssl* URI spelling.
		{
			name: "ssl URI option enables TLS with no CA file",
			uri:  "mongodb://localhost:27017/?ssl=true",
		},
		{
			name:         "sslCertificateAuthorityFile URI option is loaded into RootCAs",
			uri:          "mongodb://localhost:27017/?sslCertificateAuthorityFile=" + caFile,
			expectCAFile: caFile,
		},
		{
			name:           "sslClientCertificateKeyFile URI option loads an unencrypted client key",
			uri:            "mongodb://localhost:27017/?sslClientCertificateKeyFile=" + keyFile,
			expectCertFile: keyFile,
		},
		{
			name: "sslClientCertificateKeyPassword URI option decrypts a client key",
			uri: "mongodb://localhost:27017/?sslClientCertificateKeyFile=" + encryptedKeyFile +
				"&sslClientCertificateKeyPassword=" + keyPassword,
			expectCertFile: encryptedKeyFile,
		},
		{
			name:             "sslInsecure URI option skips verification",
			uri:              "mongodb://localhost:27017/?ssl=true&sslInsecure=true",
			expectSkipVerify: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uri := tc.uri
			if uri == "" {
				uri = "mongodb://localhost:27017/"
			}

			toolOptions := options.New("test", "", "", "", true, enabled)
			_, err := toolOptions.ParseArgs(append([]string{"--uri", uri}, tc.args...))
			require.NoError(t, err)

			tlsConfig, subject, err := buildTLSConfig(toolOptions)
			require.True(t, toolOptions.UseSSL, "SSL/TLS options enable TLS")

			if tc.expectErr != "" {
				require.ErrorContains(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion, "minimum TLS version")

			if tc.expectCAFile == "" {
				assert.Nil(t, tlsConfig.RootCAs, "no CA file leaves RootCAs nil")
			} else {
				assert.ElementsMatch(
					t,
					certPoolSubjects(t, tc.expectCAFile),
					tlsConfig.RootCAs.Subjects(),
					"RootCAs holds the certificates from %#q",
					tc.expectCAFile,
				)
			}

			assert.Equal(
				t,
				tc.expectSkipVerify,
				tlsConfig.InsecureSkipVerify,
				"verification is skipped only for an insecure option",
			)

			if tc.expectCertFile == "" {
				assert.Empty(t, tlsConfig.Certificates, "no client certificate is loaded")
				assert.Empty(t, subject, "no client certificate subject is returned")
			} else {
				expected := parseTestCertificate(t, tc.expectCertFile)
				require.Len(t, tlsConfig.Certificates, 1, "one client certificate is loaded")
				assert.Equal(
					t,
					expected.Raw,
					tlsConfig.Certificates[0].Certificate[0],
					"the loaded client certificate is the one in %#q",
					tc.expectCertFile,
				)
				assert.Equal(
					t,
					expected.Subject.String(),
					subject,
					"the returned subject is the loaded certificate's subject",
				)
			}
		})
	}
}

// certPoolSubjects returns the subjects of the certificates in the PEM file at path, as
// tls.Config.RootCAs.Subjects would report them.
func certPoolSubjects(t *testing.T, path string) [][]byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err, "can read %#q", path)

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(data), "%#q holds certificates", path)

	return pool.Subjects()
}

// parseTestCertificate returns the first certificate in the PEM file at path.
func parseTestCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err, "can read %#q", path)

	block, _ := pem.Decode(data)
	require.NotNil(t, block, "%#q holds a PEM block", path)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "can parse the certificate in %#q", path)

	return cert
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
