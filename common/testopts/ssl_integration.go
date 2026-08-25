// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package testopts

import (
	"path/filepath"
	"runtime"

	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

func GetSSLArgs() []string {
	sslOpts := GetSSLOptions()
	if sslOpts.UseSSL {
		return []string{
			"--ssl",
			"--sslCAFile", sslOpts.SSLCAFile,
			"--sslPEMKeyFile", sslOpts.SSLPEMKeyFile,
		}
	}
	return nil
}

// GetSSLOptions returns options presenting the test server certificate.
func GetSSLOptions() commonOpts.SSL {
	return SSLOptionsWithCert("test-server.pem")
}

// SSLOptionsWithCert returns options presenting the named certificate from common/db/testdata.
// Tests that connect as a client need one of the client certificates rather than the server one
// GetSSLOptions hands out.
func SSLOptionsWithCert(certFile string) commonOpts.SSL {
	if !testtype.HasTestType(testtype.SSLTestType) {
		return commonOpts.SSL{
			UseSSL: false,
		}
	}

	return commonOpts.SSL{
		UseSSL:        true,
		SSLCAFile:     TestDataPath("ca-ia.pem"),
		SSLPEMKeyFile: TestDataPath(certFile),
	}
}

// URIWithCert returns a URI whose ConnString carries the TLS file settings for the named
// certificate. Options built by hand rather than by GetToolOptions need this, because a ConnString
// the caller owns is also what lets it override one of those settings afterward.
func URIWithCert(certFile string) *commonOpts.URI {
	if !testtype.HasTestType(testtype.SSLTestType) {
		return &commonOpts.URI{}
	}

	return &commonOpts.URI{
		ConnString: &connstring.ConnString{
			SSLCaFileSet:                   true,
			SSLCaFile:                      TestDataPath("ca-ia.pem"),
			SSLClientCertificateKeyFileSet: true,
			SSLClientCertificateKeyFile:    TestDataPath(certFile),
		},
	}
}

// TestDataPath returns the absolute path of a file in common/db/testdata, so that a test naming a
// certificate does not depend on its own package's location.
func TestDataPath(file string) string {
	_, thisFile, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(thisFile), "..", "db", "testdata", file)
}
