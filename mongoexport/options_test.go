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

// validateParseOptions is a helper to call ParseOptions and verify the results.
// args: command line arguments
// expectSuccess: whether or not ParseOptions should succeed
// inputRp: the expected read preference on InputOptions
// toolsRp: the expected read preference on ToolOptions
func validateParseOptions(args []string, expectSuccess bool, inputRp string, toolsRp *readpref.ReadPref) func() {
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
			So(opts.ToolOptions.ReadPreference.Mode(), ShouldEqual, toolsRp.Mode())
		}
	}
}

func TestOptionsParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With ToolOptions and InputOptions", t, func() {
		Convey("Parsing with no values should leave read pref empty",
			validateParseOptions([]string{}, true, "", nil))

		Convey("Parsing with value only in command line opts",
			validateParseOptions([]string{"--readPreference", "secondary"}, true, "secondary", readpref.Secondary()))

		Convey("Specifying slaveOk should set read pref to nearest",
			validateParseOptions([]string{"--slaveOk"}, true, "nearest", readpref.Nearest()))

		Convey("Specifying slaveOk and read pref should error",
			validateParseOptions([]string{"--slaveOk", "--readPreference", "secondary"}, false, "", nil))
	})
}
