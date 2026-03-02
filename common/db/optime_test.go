// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
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

	timestamp1 := numberToTimestamp(6346129894295994369)
	timestamp2 := numberToTimestamp(6346129894295994370)
	term1 := int64(1)
	term2 := int64(2)

	t.Run("term comparison", func(t *testing.T) {
		// timestamp2 > timestamp1, but term1 < term2, so opTime1 is less than opTime2.
		t1 := OpTime{timestamp2, &term1}
		t2 := OpTime{timestamp1, &term2}
		assert.True(t, OpTimeLessThan(t1, t2))
	})

	t.Run("one term nil", func(t *testing.T) {
		// Compare only timestamps if one term is nil (timestamp1 < timestamp2).
		t1 := OpTime{timestamp1, &term1}
		t2 := OpTime{timestamp2, nil}
		assert.True(t, OpTimeLessThan(t1, t2))
	})

	t.Run("two terms nil", func(t *testing.T) {
		// Compare only timestamps if both terms are nil (timestamp1 < timestamp2).
		t1 := OpTime{timestamp1, nil}
		t2 := OpTime{timestamp2, nil}
		assert.True(t, OpTimeLessThan(t1, t2))
	})
}
