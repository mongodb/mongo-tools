// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package archive

import (
	"bytes"

	. "github.com/smartystreets/goconvey/convey"
	//	"gopkg.in/mgo.v2/bson"
	"github.com/mongodb/mongo-tools/common/testtype"
	"testing"
)

func TestPrelude(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	var err error

	Convey("WritePrelude/ReadPrelude roundtrip", t, func() {

		cm1 := &CollectionMetadata{
			Database:   "db1",
			Collection: "c1",
			Metadata:   "m1",
		}
		cm2 := &CollectionMetadata{
			Database:   "db1",
			Collection: "c2",
			Metadata:   "m2",
		}
		cm3 := &CollectionMetadata{
			Database:   "db2",
			Collection: "c3",
			Metadata:   "m3",
		}
		cm4 := &CollectionMetadata{
			Database:   "db3",
			Collection: "c4",
			Metadata:   "m4",
		}

		archivePrelude := &Prelude{
			Header: &Header{
				FormatVersion: "version-foo",
			},
			NamespaceMetadatas: []*CollectionMetadata{cm1, cm2, cm3, cm4},
			DBS:                []string{"db1", "db2", "db3"},
			NamespaceMetadatasByDB: map[string][]*CollectionMetadata{
				"db1": []*CollectionMetadata{cm1, cm2},
				"db2": []*CollectionMetadata{cm3},
				"db3": []*CollectionMetadata{cm4},
			},
		}
		buf := &bytes.Buffer{}
		err = archivePrelude.Write(buf)
		So(err, ShouldBeNil)
		archivePrelude2 := &Prelude{}
		err := archivePrelude2.Read(buf)
		So(err, ShouldBeNil)
		So(archivePrelude2, ShouldResemble, archivePrelude)
	})
}
