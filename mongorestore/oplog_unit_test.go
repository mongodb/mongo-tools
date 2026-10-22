// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongorestore

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestTimestampStringParsing(t *testing.T) {
	Convey("Testing some possible timestamp strings:", t, func() {
		Convey("123:456 [should pass]", func() {
			ts, err := ParseTimestampFlag("123:456")
			So(err, ShouldBeNil)
			So(ts, ShouldResemble, bson.Timestamp{T: 123, I: 456})
		})

		Convey("123 [should pass]", func() {
			ts, err := ParseTimestampFlag("123")
			So(err, ShouldBeNil)
			So(ts, ShouldResemble, bson.Timestamp{T: 123, I: 0})
		})

		Convey("123: [should pass]", func() {
			ts, err := ParseTimestampFlag("123:")
			So(err, ShouldBeNil)
			So(ts, ShouldResemble, bson.Timestamp{T: 123, I: 0})
		})

		Convey("123.123 [should fail]", func() {
			ts, err := ParseTimestampFlag("123.123")
			So(err, ShouldNotBeNil)
			So(ts, ShouldResemble, bson.Timestamp{})
		})

		Convey(": [should fail]", func() {
			ts, err := ParseTimestampFlag(":")
			So(err, ShouldNotBeNil)
			So(ts, ShouldResemble, bson.Timestamp{})
		})

		Convey("1:1:1 [should fail]", func() {
			ts, err := ParseTimestampFlag("1:1:1")
			So(err, ShouldNotBeNil)
			So(ts, ShouldResemble, bson.Timestamp{})
		})

		Convey("cats [should fail]", func() {
			ts, err := ParseTimestampFlag("cats")
			So(err, ShouldNotBeNil)
			So(ts, ShouldResemble, bson.Timestamp{})
		})

		Convey("[empty string] [should fail]", func() {
			ts, err := ParseTimestampFlag("")
			So(err, ShouldNotBeNil)
			So(ts, ShouldResemble, bson.Timestamp{})
		})
	})
}

func TestValidOplogLimitChecking(t *testing.T) {
	Convey("With a MongoRestore instance with oplogLimit of 5:0", t, func() {
		mr := &MongoRestore{
			oplogLimit: bson.Timestamp{T: 5, I: 0},
		}

		Convey("an oplog entry with ts=1000:0 should be invalid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 1000, I: 0}), ShouldBeFalse)
		})

		Convey("an oplog entry with ts=5:1 should be invalid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 5, I: 1}), ShouldBeFalse)
		})

		Convey("an oplog entry with ts=5:0 should be invalid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 5, I: 0}), ShouldBeFalse)
		})

		Convey("an oplog entry with ts=4:9 should be valid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 4, I: 9}), ShouldBeTrue)
		})

		Convey("an oplog entry with ts=4:0 should be valid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 4, I: 0}), ShouldBeTrue)
		})

		Convey("an oplog entry with ts=0:1 should be valid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 0, I: 1}), ShouldBeTrue)
		})
	})

	Convey("With a MongoRestore instance with no oplogLimit", t, func() {
		mr := &MongoRestore{}

		Convey("an oplog entry with ts=1000:0 should be valid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 1000, I: 0}), ShouldBeTrue)
		})

		Convey("an oplog entry with ts=5:1 should be valid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 5, I: 1}), ShouldBeTrue)
		})

		Convey("an oplog entry with ts=5:0 should be valid", func() {
			So(mr.TimestampBeforeLimit(bson.Timestamp{T: 5, I: 0}), ShouldBeTrue)
		})
	})

}

type testTable struct {
	ns     string
	output bool
}

func TestShouldIgnoreNamespace(t *testing.T) {
	tests := []testTable{
		{
			ns:     "test.system",
			output: false,
		},
		{
			ns:     "test.system.nonsense",
			output: false,
		},
		{
			ns:     "config.system.sessions",
			output: true,
		},
		{
			ns:     "config.system.indexBuilds",
			output: true,
		},
		{
			ns:     "config.system.preimages",
			output: true,
		},
		{
			ns:     "config.transactions",
			output: true,
		},
		{
			ns:     "config.transaction_coordinators",
			output: true,
		},
		{
			ns:     "config.system.sharding_ddl_coordinators",
			output: true,
		},
		{
			ns:     "config.image_collection",
			output: true,
		},
		{
			ns:     "config.mongos",
			output: true,
		},
		{
			ns:     "test.system.js",
			output: false,
		},
		{
			ns:     "test.test",
			output: false,
		},
		{
			ns:     "config.cache.any",
			output: true,
		},
	}
	for _, testVals := range tests {
		if shouldIgnoreNamespace(testVals.ns) != testVals.output {
			t.Errorf("%s should have been %v but failed\n", testVals.ns, testVals.output)
		}
	}
}

func TestIsCompressedBucketDiff(t *testing.T) {
	tests := []struct {
		name string
		op   db.Oplog
		want bool
	}{
		{
			name: "timeseries bucket append",
			op: db.Oplog{
				Operation: "u",
				Object: bson.D{
					{"$v", int32(2)},
					{
						"diff",
						bson.D{
							{"scontrol", bson.D{{"u", bson.D{{"count", int32(4)}}}}},
							{
								"sdata",
								bson.D{
									{
										"b",
										bson.D{
											{
												"ts",
												bson.D{
													{"o", int32(0)},
													{"d", bson.Binary{Data: []byte{1, 2, 3}}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "ordinary v2 update",
			op: db.Oplog{
				Operation: "u",
				Object: bson.D{
					{"$v", int32(2)},
					{"diff", bson.D{{"u", bson.D{{"x", int32(1)}}}}},
				},
			},
			want: false,
		},
		{
			name: "ordinary v2 update with a sub-diff",
			op: db.Oplog{
				Operation: "u",
				Object: bson.D{
					{"$v", int32(2)},
					{"diff", bson.D{{"sfoo", bson.D{{"u", bson.D{{"x", int32(1)}}}}}}},
				},
			},
			want: false,
		},
		{
			// A regular update to a field named "control" produces an "scontrol" section too, but
			// no "sdata", so it must not be treated as a bucket append.
			name: "v2 update touching a field named control",
			op: db.Oplog{
				Operation: "u",
				Object: bson.D{
					{"$v", int32(2)},
					{"diff", bson.D{{"scontrol", bson.D{{"u", bson.D{{"x", int32(1)}}}}}}},
				},
			},
			want: false,
		},
		{
			// A regular update to a field named "data" produces an "sdata" section too, but no
			// "scontrol", so it must not be treated as a bucket append.
			name: "v2 update touching a field named data",
			op: db.Oplog{
				Operation: "u",
				Object: bson.D{
					{"$v", int32(2)},
					{"diff", bson.D{{"sdata", bson.D{{"u", bson.D{{"x", int32(1)}}}}}}},
				},
			},
			want: false,
		},
		{
			name: "v1 update",
			op: db.Oplog{
				Operation: "u",
				Object:    bson.D{{"$set", bson.D{{"x", int32(1)}}}},
			},
			want: false,
		},
		{
			name: "insert",
			op: db.Oplog{
				Operation: "i",
				Object:    bson.D{{"_id", int32(1)}},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(
				t,
				tc.want,
				isCompressedBucketDiff(tc.op),
				"isCompressedBucketDiff(%v)",
				tc.op,
			)
		})
	}
}
