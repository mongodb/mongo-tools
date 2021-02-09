// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func numberToTimestamp(ts int64) primitive.Timestamp {
	return primitive.Timestamp{
		T: uint32(uint64(ts) >> 32),
		I: uint32(ts),
	}
}

func TestOpTimeComparisons(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When comparing two OpTimes", t, func() {

		var opTime1, opTime2 OpTime
		var timestamp1, timestamp2 primitive.Timestamp
		var term1, term2 int64
		var hash1, hash2 int64

		Convey("Equality should be true if all optime fields match", func() {
			timestamp1 = numberToTimestamp(6346129894295994369)
			timestamp2 = numberToTimestamp(6346129894295994369)
			term1 = 1
			term2 = 1
			hash1 = 0
			hash2 = 0

			// Nothing is nil.
			opTime1 = OpTime{timestamp1, &term1, &hash1}
			opTime2 = OpTime{timestamp2, &term2, &hash2}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeTrue)

			// Terms are nil.
			opTime1 = OpTime{timestamp1, nil, &hash1}
			opTime2 = OpTime{timestamp2, nil, &hash2}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeTrue)

			// Hashes are nil.
			opTime1 = OpTime{timestamp1, &term1, nil}
			opTime2 = OpTime{timestamp2, &term2, nil}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeTrue)

			// Terms and hashes are nil.
			opTime1 = OpTime{timestamp1, nil, nil}
			opTime2 = OpTime{timestamp2, nil, nil}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeTrue)
		})

		Convey("Equality should be false if any optime fields don't match", func() {
			timestamp1 = numberToTimestamp(6346129894295994369)
			timestamp2 = numberToTimestamp(6346129894295994369)
			term1 = 1
			term2 = 1
			hash1 = 0
			hash2 = 0

			// One term is nil but the other isn't.
			opTime1 = OpTime{timestamp1, nil, &hash1}
			opTime2 = OpTime{timestamp2, &term2, &hash2}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeFalse)

			// One hash is nil but the other isn't.
			opTime1 = OpTime{timestamp1, &term1, nil}
			opTime2 = OpTime{timestamp2, &term2, &hash2}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeFalse)

			// Assign different values to all fields.
			timestamp1 = numberToTimestamp(6346129894295994369)
			timestamp2 = numberToTimestamp(6346129894295994370)
			term1 = 1
			term2 = 2
			hash1 = 0
			hash1 = 1

			// None of the values are equal.
			opTime1 = OpTime{timestamp1, &term1, &hash1}
			opTime2 = OpTime{timestamp2, &term2, &hash2}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeFalse)

			// Both terms and hashes are nil, but timestamps still differ.
			opTime1 = OpTime{timestamp1, nil, nil}
			opTime2 = OpTime{timestamp2, nil, nil}
			So(OpTimeEquals(opTime1, opTime2), ShouldBeFalse)
		})

		Convey("Less than should be true if one optime precedes the other", func() {
			timestamp1 = numberToTimestamp(6346129894295994369)
			timestamp2 = numberToTimestamp(6346129894295994370)
			term1 = 1
			term2 = 2

			// timestamp2 > timestamp1, but term1 < term2, so opTime1 is less than opTime2.
			opTime1 = OpTime{timestamp2, &term1, nil}
			opTime2 = OpTime{timestamp1, &term2, nil}
			So(OpTimeLessThan(opTime1, opTime2), ShouldBeTrue)

			// Compare only timestamps if one term is nil (timestamp1 < timestamp2).
			opTime1 = OpTime{timestamp1, &term1, nil}
			opTime2 = OpTime{timestamp2, nil, nil}
			So(OpTimeLessThan(opTime1, opTime2), ShouldBeTrue)

			// Compare only timestamps if both terms are nil (timestamp1 < timestamp2).
			opTime1 = OpTime{timestamp1, nil, nil}
			opTime2 = OpTime{timestamp2, nil, nil}
			So(OpTimeLessThan(opTime1, opTime2), ShouldBeTrue)
		})

		Convey("Greater than should be true if one optime comes after the other", func() {
			timestamp1 = numberToTimestamp(6346129894295994369)
			timestamp2 = numberToTimestamp(6346129894295994370)
			term1 = 1
			term2 = 2

			// timestamp1 < timestamp2, but term2 > term1, so opTime1 is greater than opTime2.
			opTime1 = OpTime{timestamp1, &term2, nil}
			opTime2 = OpTime{timestamp2, &term1, nil}
			So(OpTimeGreaterThan(opTime1, opTime2), ShouldBeTrue)

			// Compare only timestamps if one term is nil (timestamp2 > timestamp1).
			opTime1 = OpTime{timestamp2, &term1, nil}
			opTime2 = OpTime{timestamp1, nil, nil}
			So(OpTimeGreaterThan(opTime1, opTime2), ShouldBeTrue)

			// Compare only timestamps if both terms are nil (timestamp2 > timestamp1).
			opTime1 = OpTime{timestamp2, nil, nil}
			opTime2 = OpTime{timestamp1, nil, nil}
			So(OpTimeGreaterThan(opTime1, opTime2), ShouldBeTrue)
		})
	})
}
