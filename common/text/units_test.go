// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package text

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
)

func TestFormatByteCount(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tests := []struct {
		size   int64
		expect string
	}{
		{0, "0B"},
		{1024, "1.00KB"},
		{2500, "2.44KB"},
		{2 * 1024 * 1024, "2.00MB"},
		{5 * 1024 * 1024 * 1024, "5.00GB"},
		{5 * 1024 * 1024 * 1024 * 1024, "5120GB"},
	}

	for _, test := range tests {
		got := FormatByteAmount(test.size)
		assert.Equal(t, test.expect, got, "%d -> %s", test.size, test.expect)
	}
}

func TestOtherByteFormats(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	val := int64(10)
	assert.Equal(t, "10.0M", FormatMegabyteAmount(val))
	assert.Equal(t, "10B", FormatByteAmount(val))
	assert.Equal(t, "10b", FormatBits(val))

	val = int64(2.5 * 1024)
	assert.Equal(t, "2.50G", FormatMegabyteAmount(val))
	assert.Equal(t, "2.50KB", FormatByteAmount(val))
	assert.Equal(t, "2.56k", FormatBits(val))
}

func TestBitFormatPrecision(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tests := []struct {
		size   int64
		expect string
	}{
		{999, "999b"},
		{99, "99b"},
		{9, "9b"},

		{9999, "10.0k"},
		{9990, "9.99k"},

		{999_000_000, "999m"},
		{9_990_000_000, "9.99g"},
	}

	for _, test := range tests {
		got := FormatBits(test.size)
		assert.Equal(t, test.expect, got, "%d -> %s", test.size, test.expect)
	}

	/*
				t.Run("values less than 1k", func(t *testing.T) {

					Convey("with '999'", func() {
						Convey("FormatBits -> 999b", func() {
							So(FormatBits(999), ShouldEqual, "999b")
						})
					})
					Convey("with '99'", func() {
						Convey("FormatBits -> 99b", func() {
							So(FormatBits(99), ShouldEqual, "99b")
						})
					})
					Convey("with '9'", func() {
						Convey("FormatBits -> 9b", func() {
							So(FormatBits(9), ShouldEqual, "9b")
						})
					})
				})

			t.Run("values less than 1m", func(t *testing.T) {
				Convey("with '9999'", func() {
					Convey("FormatBits -> 10.0k", func() {
						So(FormatBits(9999), ShouldEqual, "10.0k")
					})
				})
				Convey("with '9990'", func() {
					Convey("FormatBits -> 9.99k", func() {
						So(FormatBits(9990), ShouldEqual, "9.99k")
					})
				})
			})

		t.Run("big numbers", func(t *testing.T) {
			Convey("with '999000000'", func() {
				Convey("FormatBits -> 999m", func() {
					So(FormatBits(999000000), ShouldEqual, "999m")
				})
			})
			Convey("with '9990000000'", func() {
				Convey("FormatBits -> 9.99g", func() {
					So(FormatBits(9990000000), ShouldEqual, "9.99g")
				})
			})
		})
	*/
}
