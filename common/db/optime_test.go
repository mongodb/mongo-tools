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
	"go.mongodb.org/mongo-driver/v2/bson"
)

func numberToTimestamp(ts int64) bson.Timestamp {
	return bson.Timestamp{
		T: uint32(uint64(ts) >> 32),
		I: uint32(ts),
	}
}

func TestOpTimeComparisons(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("When comparing two OpTimes", t, func() {

		var opTime1, opTime2 OpTime
		var timestamp1, timestamp2 bson.Timestamp
		var term1, term2 int64

		Convey("Less than should be true if one optime precedes the other", func() {
			timestamp1 = numberToTimestamp(6346129894295994369)
			timestamp2 = numberToTimestamp(6346129894295994370)
			term1 = 1
			term2 = 2

			// timestamp2 > timestamp1, but term1 < term2, so opTime1 is less than opTime2.
			opTime1 = OpTime{timestamp2, &term1}
			opTime2 = OpTime{timestamp1, &term2}
			So(OpTimeLessThan(opTime1, opTime2), ShouldBeTrue)

			// Compare only timestamps if one term is nil (timestamp1 < timestamp2).
			opTime1 = OpTime{timestamp1, &term1}
			opTime2 = OpTime{timestamp2, nil}
			So(OpTimeLessThan(opTime1, opTime2), ShouldBeTrue)

			// Compare only timestamps if both terms are nil (timestamp1 < timestamp2).
			opTime1 = OpTime{timestamp1, nil}
			opTime2 = OpTime{timestamp2, nil}
			So(OpTimeLessThan(opTime1, opTime2), ShouldBeTrue)
		})
	})
}
