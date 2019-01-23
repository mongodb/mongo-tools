// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db_test

// This file runs Kerberos tests if the test.types includes 'kerberos'

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var (
	KERBEROS_HOST = "ldaptest.10gen.cc"
	KERBEROS_USER = "drivers@LDAPTEST.10GEN.CC"
)

func TestKerberosAuthMechanism(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.KerberosTestType)

	Convey("should be able to successfully connect", t, func() {
		connector := &db.VanillaDBConnector{}

		opts := options.ToolOptions{
			Connection: &options.Connection{
				Host: KERBEROS_HOST,
				Port: "27017",
			},
			Auth: &options.Auth{
				Username:  KERBEROS_USER,
				Mechanism: "GSSAPI",
			},
			Kerberos: &options.Kerberos{
				Service:     "mongodb",
				ServiceHost: KERBEROS_HOST,
			},
		}

		if runtime.GOOS == "windows" {
			opts.Auth.Password = os.Getenv(testutil.WinKerberosPwdEnv)
			if opts.Auth.Password == "" {
				panic(fmt.Sprintf("Need to set %v environment variable to run kerberos tests on windows",
					testutil.WinKerberosPwdEnv))
			}
		}

		So(connector.Configure(opts), ShouldBeNil)

		session, err := connector.GetNewSession()
		So(err, ShouldBeNil)
		So(session, ShouldNotBeNil)

		n, err := session.DB("kerberos").C("test").Find(bson.M{}).Count()
		So(err, ShouldBeNil)
		So(n, ShouldEqual, 1)
	})
}
