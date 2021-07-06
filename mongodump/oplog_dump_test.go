// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"io/ioutil"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
)

func TestErrorOnImportCollection(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("An importCollection oplog entry should error", t, func() {
		rawOp, err := ioutil.ReadFile("./testdata/importCollection.bson")
		So(err, ShouldBeNil)

		err = oplogDocumentValidator(rawOp)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "cannot dump with oplog while importCollection occurs")
	})
}
