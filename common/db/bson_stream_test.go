// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"bytes"
	"io"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBufferlessBSONSource(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	var testValues = []bson.M{
		{"_": primitive.Binary{Subtype: 0x80, Data: []byte("apples")}},
		{"_": primitive.Binary{Subtype: 0x80, Data: []byte("bananas")}},
		{"_": primitive.Binary{Subtype: 0x80, Data: []byte("cherries")}},
	}
	Convey("with a buffer containing several bson documents with binary fields", t, func() {
		writeBuf := bytes.NewBuffer(make([]byte, 0, 1024))
		for _, tv := range testValues {
			data, err := bson.Marshal(&tv)
			So(err, ShouldBeNil)
			_, err = writeBuf.Write(data)
			So(err, ShouldBeNil)
		}
		Convey("that we parse correctly with a BufferlessBSONSource", func() {
			bsonSource := NewDecodedBSONSource(
				NewBufferlessBSONSource(io.NopCloser(writeBuf)))
			docs := []bson.M{}
			count := 0
			doc := &bson.M{}
			for bsonSource.Next(doc) {
				count++
				docs = append(docs, *doc)
				doc = &bson.M{}
			}
			So(bsonSource.Err(), ShouldBeNil)
			So(count, ShouldEqual, len(testValues))
			So(docs, ShouldResemble, testValues)
		})
	})
}
