// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"testing"

	"github.com/mongodb/mongo-tools-common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// validateReadPreferenceParsing is a helper to call ParseOptions and verify the results for read preferences.
// args: command line arguments
// expectSuccess: whether or not ParseOptions should succeed
// inputRp: the expected read preference on InputOptions
// toolsRp: the expected read preference on ToolOptions
func validateReadPreferenceParsing(args []string, expectSuccess bool, inputRp string, toolsRp *readpref.ReadPref) func() {
	return func() {
		opts, err := ParseOptions(args)
		if expectSuccess {
			So(err, ShouldBeNil)
		} else {
			So(err, ShouldNotBeNil)
			return
		}

		So(opts.InputOptions.ReadPreference, ShouldEqual, inputRp)
		if toolsRp == nil {
			So(opts.ToolOptions.ReadPreference, ShouldBeNil)
		} else {
			So(opts.ToolOptions.ReadPreference, ShouldNotBeNil)
			So(opts.ToolOptions.ReadPreference.Mode(), ShouldEqual, toolsRp.Mode())
		}
	}
}

func TestReadPreferenceParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With ToolOptions and InputOptions", t, func() {
		Convey("Parsing with no values should default to primary",
			validateReadPreferenceParsing([]string{}, true, "", readpref.Primary()))

		Convey("Parsing with value only in command line opts should set read pref correctly",
			validateReadPreferenceParsing([]string{"--readPreference", "secondary"}, true, "secondary", readpref.Secondary()))

		Convey("Parsing with value only in URI should set read pref correctly",
			validateReadPreferenceParsing([]string{"--uri", "mongodb://localhost:27017/db?readPreference=secondary"},
				true, "", readpref.Secondary()))

		Convey("Specifying slaveOk should set read pref to nearest",
			validateReadPreferenceParsing([]string{"--slaveOk"}, true, "nearest", readpref.Nearest()))

		Convey("Specifying slaveOk and read pref should error",
			validateReadPreferenceParsing([]string{"--slaveOk", "--readPreference", "secondary"}, false, "", nil))
	})
}
