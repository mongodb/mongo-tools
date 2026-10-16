// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build kerberos

package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/stretchr/testify/require"
)

const (
	kerberosUsername   = "drivers%40LDAPTEST.10GEN.CC"
	kerberosConnection = "ldaptest.10gen.cc:27017"
)

func TestKerberosAuthConnection(t *testing.T) {
	enabled := options.EnabledOptions{URI: true}

	uri := "mongodb://" + kerberosUsername + "@" + kerberosConnection + "/kerberos?authSource=$external&authMechanism=GSSAPI"

	fakeArgs := []string{"--uri=" + uri}
	toolOptions := options.New("test", "", "", "", true, enabled)
	_, err := toolOptions.ParseArgs(fakeArgs)
	if err != nil {
		panic("Could not parse MONGODB_URI file contents")
	}

	_, err = NewSessionProvider(*toolOptions)
	require.NoError(t, err, "connection succeeds")
}
