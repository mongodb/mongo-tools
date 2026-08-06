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
}
