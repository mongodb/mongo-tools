// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
)

func TestBufferedBulkInserterInserts(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	var bufBulk *BufferedBulkInserter

	auth := DBGetAuthOptions()
	ssl := DBGetSSLOptions()
	Convey("With a valid session", t, func() {
		opts := options.ToolOptions{
			Connection: &options.Connection{
				Port: DefaultTestPort,
			},
			URI:  &options.URI{},
			SSL:  &ssl,
			Auth: &auth,
		}
		err := opts.NormalizeOptionsAndURI()
		So(err, ShouldBeNil)
		provider, err := NewSessionProvider(opts)
		So(provider, ShouldNotBeNil)
		So(err, ShouldBeNil)
		session, err := provider.GetSession()
		So(session, ShouldNotBeNil)
		So(err, ShouldBeNil)

		Convey("using a test collection and a doc limit of 3", func() {
			testCol := session.Database("tools-test").Collection("bulk1")
			bufBulk = NewUnorderedBufferedBulkInserter(testCol, 3)
			So(bufBulk, ShouldNotBeNil)

			Convey("inserting 10 documents into the BufferedBulkInserter", func() {
				flushCount := 0
				for i := 0; i < 10; i++ {
					result, err := bufBulk.Insert(bson.D{})
					So(err, ShouldBeNil)
					if bufBulk.docCount%3 == 0 {
						flushCount++
						So(result, ShouldNotBeNil)
						So(result.InsertedCount, ShouldEqual, 3)
					} else {
						So(result, ShouldBeNil)
					}
				}

				Convey("should have flushed 3 times with one doc still buffered", func() {
					So(flushCount, ShouldEqual, 3)
					So(bufBulk.docCount, ShouldEqual, 1)
				})
			})
		})

		Convey("using a test collection and a doc limit of 1", func() {
			testCol := session.Database("tools-test").Collection("bulk2")
			bufBulk = NewUnorderedBufferedBulkInserter(testCol, 1)
			So(bufBulk, ShouldNotBeNil)

			Convey("inserting 10 documents into the BufferedBulkInserter and flushing", func() {
				for i := 0; i < 10; i++ {
					result, err := bufBulk.Insert(bson.D{})
					So(err, ShouldBeNil)
					So(result, ShouldNotBeNil)
					So(result.InsertedCount, ShouldEqual, 1)
				}
				result, err := bufBulk.Flush()
				So(err, ShouldBeNil)
				So(result, ShouldBeNil)

				Convey("should have no docs buffered", func() {
					So(bufBulk.docCount, ShouldEqual, 0)
				})
			})
		})

		Convey("using a test collection and a doc limit of 1000", func() {
			testCol := session.Database("tools-test").Collection("bulk3")
			bufBulk = NewUnorderedBufferedBulkInserter(testCol, 100)
			So(bufBulk, ShouldNotBeNil)

			Convey("inserting 1,000,000 documents into the BufferedBulkInserter and flushing", func() {

				errCnt := 0
				for i := 0; i < 1000000; i++ {
					result, err := bufBulk.Insert(bson.M{"_id": i})
					if err != nil {
						errCnt++
					}
					if (i+1)%10000 == 0 {
						So(result, ShouldNotBeNil)
						So(result.InsertedCount, ShouldEqual, 100)
					}
				}
				So(errCnt, ShouldEqual, 0)
				_, err := bufBulk.Flush()
				So(err, ShouldBeNil)

				Convey("should have inserted all of the documents", func() {
					count, err := testCol.CountDocuments(context.Background(), bson.M{})
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 1000000)

					// test values
					testDoc := bson.M{}
					result := testCol.FindOne(context.Background(), bson.M{"_id": 477232})
					err = result.Decode(&testDoc)
					So(err, ShouldBeNil)
					So(testDoc["_id"], ShouldEqual, 477232)
					result = testCol.FindOne(context.Background(), bson.M{"_id": 999999})
					err = result.Decode(&testDoc)
					So(err, ShouldBeNil)
					So(testDoc["_id"], ShouldEqual, 999999)
					result = testCol.FindOne(context.Background(), bson.M{"_id": 1})
					err = result.Decode(&testDoc)
					So(err, ShouldBeNil)
					So(testDoc["_id"], ShouldEqual, 1)

				})
			})
		})

		Convey("using a test collection and a byte limit of 1", func() {
			testCol := session.Database("tools-test").Collection("bulk4")
			bufBulk = NewUnorderedBufferedBulkInserter(testCol, 1000)
			So(bufBulk, ShouldNotBeNil)
			bufBulk.byteLimit = 1

			Convey("inserting 10 documents into the BufferedBulkInserter", func() {
				for i := 0; i < 10; i++ {
					result, err := bufBulk.Insert(bson.D{{"foo", "bar"}})
					So(err, ShouldBeNil)
					So(result, ShouldNotBeNil)
					So(result.InsertedCount, ShouldEqual, 1)
				}
			})
		})

		Reset(func() {
			So(provider.DropDatabase("tools-test"), ShouldBeNil)
			provider.Close()
		})
	})

}
