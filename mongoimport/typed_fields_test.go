// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func init() {
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

func TestTypedHeaderParser(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("using zip.string(),number.double(),foo.auto(),bar.date(...)", func(t *testing.T) {
		headers := []string{
			"zip.string()",
			"number.double()",
			"foo.auto()",
			`bar.date(January 2\, \(2006\))`,
		}
		bar := FieldDateParser{"January 2, (2006)"}

		graceCases := []struct {
			name     string
			grace    ParseGrace
			expected []ColumnSpec
		}{
			{
				name:  "with parse grace: auto",
				grace: pgAutoCast,
				expected: []ColumnSpec{
					{"zip", new(FieldStringParser), pgAutoCast, "string", []string{"zip"}},
					{"number", new(FieldDoubleParser), pgAutoCast, "double", []string{"number"}},
					{"foo", new(FieldAutoParser), pgAutoCast, "auto", []string{"foo"}},
					{"bar", &bar, pgAutoCast, "date", []string{"bar"}},
				},
			},
			{
				name:  "with parse grace: skipRow",
				grace: pgSkipRow,
				expected: []ColumnSpec{
					{"zip", new(FieldStringParser), pgSkipRow, "string", []string{"zip"}},
					{"number", new(FieldDoubleParser), pgSkipRow, "double", []string{"number"}},
					{"foo", new(FieldAutoParser), pgSkipRow, "auto", []string{"foo"}},
					{"bar", &bar, pgSkipRow, "date", []string{"bar"}},
				},
			},
		}

		for _, tc := range graceCases {
			colSpecs, err := ParseTypedHeaders(headers, tc.grace)
			assert.Equal(t, tc.expected, colSpecs, "should parse the headers %s", tc.name)
			assert.NoError(t, err, "should parse the headers without error %s", tc.name)
		}
	})

	t.Run("using various bad headers", func(t *testing.T) {
		nonEmptyArgHeaders := []string{
			"zip.string(blah)",
			"zip.string(0)",
			"zip.int32(0)",
			"zip.int64(0)",
			"zip.double(0)",
			"zip.auto(0)",
		}
		for _, header := range nonEmptyArgHeaders {
			_, err := ParseTypedHeader(header, pgAutoCast)
			assert.Error(t, err, "should reject the non-empty argument in %q", header)
		}

		badBinaryArgHeaders := []string{
			"zip.binary(blah)",
			"zip.binary(binary)",
			"zip.binary(decimal)",
		}
		for _, header := range badBinaryArgHeaders {
			_, err := ParseTypedHeader(header, pgAutoCast)
			assert.Error(t, err, "should reject the bad binary argument in %q", header)
		}
	})
}

func TestAutoHeaderParser(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	headers := []string{"zip", "number", "foo"}
	colSpecs := ParseAutoHeaders(headers)
	expected := []ColumnSpec{
		{"zip", new(FieldAutoParser), pgAutoCast, "auto", []string{"zip"}},
		{"number", new(FieldAutoParser), pgAutoCast, "auto", []string{"number"}},
		{"foo", new(FieldAutoParser), pgAutoCast, "auto", []string{"foo"}},
	}
	require.Equal(t, expected, colSpecs, "should build a column spec for every header")
}

func TestFieldParsers(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("using fieldautoparser", func(t *testing.T) {
		p, _ := NewFieldParser(ctAuto, "")

		value, err := p.Parse("2147483648")
		assert.Equal(t, int64(2147483648), cast[int64](t, value), "should parse a large integer")
		assert.NoError(t, err, "should parse a large integer without error")
		value, err = p.Parse("42")
		assert.Equal(t, int32(42), cast[int32](t, value), "should parse a small integer")
		assert.NoError(t, err, "should parse a small integer without error")
		value, _ = p.Parse("-2147483649")
		assert.Equal(
			t,
			int64(-2147483649),
			cast[int64](t, value),
			"should parse a large negative integer",
		)

		decimalCases := []struct {
			in       string
			expected float64
		}{
			{"3.14159265", 3.14159265},
			{"0.123123", 0.123123},
			{"-123456.789", -123456.789},
			{"-1.", -1.0},
		}
		for _, tc := range decimalCases {
			value, err := p.Parse(tc.in)
			assert.Equal(t, tc.expected, cast[float64](t, value), "should parse %q", tc.in)
			assert.NoError(t, err, "should parse %q without error", tc.in)
		}

		stringCases := []struct {
			in       string
			expected string
		}{
			{"12345-6789", "12345-6789"},
			{"06/02/1997", "06/02/1997"},
			{"", ""},
		}
		for _, tc := range stringCases {
			value, err := p.Parse(tc.in)
			assert.Equal(t, tc.expected, cast[string](t, value), "should fall back to %q", tc.in)
			assert.NoError(t, err, "should parse %q without error", tc.in)
		}
	})

	t.Run("using fieldbooleanparser", func(t *testing.T) {
		p, _ := NewFieldParser(ctBoolean, "")

		trueCases := []string{"true", "TrUe", "1"}
		for _, in := range trueCases {
			value, err := p.Parse(in)
			assert.True(t, cast[bool](t, value), "should parse %q as true", in)
			assert.NoError(t, err, "should parse %q without error", in)
		}

		falseCases := []string{"false", "FaLsE", "0"}
		for _, in := range falseCases {
			value, err := p.Parse(in)
			assert.False(t, cast[bool](t, value), "should parse %q as false", in)
			assert.NoError(t, err, "should parse %q without error", in)
		}

		invalidCases := []string{"", "t", "f", "yes", "no"}
		for _, in := range invalidCases {
			_, err := p.Parse(in)
			assert.Error(t, err, "should reject %q as a boolean", in)
		}
	})

	t.Run("using fieldbinaryparser", func(t *testing.T) {
		type binaryCase struct {
			in       string
			expected []byte
		}

		encodingCases := []struct {
			name     string
			encoding string
			cases    []binaryCase
		}{
			{
				name:     "using hex encoding",
				encoding: "hex",
				cases: []binaryCase{
					{"400a11", []byte{64, 10, 17}},
					{"400A11", []byte{64, 10, 17}},
					{"0b400A11", []byte{11, 64, 10, 17}},
					{"", []byte{}},
				},
			},
			{
				name:     "using base32 encoding",
				encoding: "base32",
				cases: []binaryCase{
					{"", []byte{}},
					{"MZXW6YTBOI======", []byte{102, 111, 111, 98, 97, 114}},
				},
			},
			{
				name:     "using base64 encoding",
				encoding: "base64",
				cases: []binaryCase{
					{"", []byte{}},
					{"Zm9vYmFy", []byte{102, 111, 111, 98, 97, 114}},
				},
			},
		}

		for _, tc := range encodingCases {
			t.Run(tc.name, func(t *testing.T) {
				p, _ := NewFieldParser(ctBinary, tc.encoding)

				for _, c := range tc.cases {
					value, err := p.Parse(c.in)
					assert.Equal(
						t,
						c.expected,
						cast[[]byte](t, value),
						"should decode %q as %s",
						c.in,
						tc.encoding,
					)
					assert.NoError(t, err, "should decode %q without error", c.in)
				}
			})
		}
	})

	t.Run("using fielddateparser", func(t *testing.T) {
		formatCases := []struct {
			name       string
			columnType columnType
			format     string
			validInput string
			expected   time.Time
			invalid    []string
		}{
			{
				name:       "with Go's format",
				columnType: ctDateGo,
				format:     "01/02/2006 3:04:05pm MST",
				validInput: "01/04/2000 5:38:10pm UTC",
				expected:   time.Date(2000, 1, 4, 17, 38, 10, 0, time.UTC),
				invalid: []string{
					"01/04/2000 5:38:10pm",
					"01/04/2000 5:38:10 pm UTC",
					"01/04/2000",
				},
			},
			{
				name:       "with MS's format",
				columnType: ctDateMS,
				format:     "MM/dd/yyyy h:mm:sstt",
				validInput: "01/04/2000 5:38:10PM",
				expected:   time.Date(2000, 1, 4, 17, 38, 10, 0, time.UTC),
				invalid: []string{
					"01/04/2000 :) 05:38:10PM",
					"01/04/2000 005:38:10PM",
					"01/04/2000 5:38:10 PM",
					"01/04/2000",
				},
			},
			{
				name:       "with Oracle's format",
				columnType: ctDateOracle,
				format:     "mm/Dd/yYYy hh:MI:SsAm",
				validInput: "01/04/2000 05:38:10PM",
				expected:   time.Date(2000, 1, 4, 17, 38, 10, 0, time.UTC),
				invalid: []string{
					"01/04/2000 :) 05:38:10PM",
					"01/04/2000 005:38:10PM",
					"01/04/2000 5:38:10 PM",
					"01/04/2000",
				},
			},
		}

		for _, tc := range formatCases {
			t.Run(tc.name, func(t *testing.T) {
				p, _ := NewFieldParser(tc.columnType, tc.format)

				value, err := p.Parse(tc.validInput)
				assert.Equal(
					t,
					tc.expected,
					cast[time.Time](t, value),
					"should parse %q",
					tc.validInput,
				)
				assert.NoError(t, err, "should parse %q without error", tc.validInput)

				for _, in := range tc.invalid {
					_, err := p.Parse(in)
					assert.Error(t, err, "should reject %q as a timestamp", in)
				}
			})
		}
	})

	t.Run("using fielddoubleparser", func(t *testing.T) {
		p, _ := NewFieldParser(ctDouble, "")

		validCases := []struct {
			in       string
			expected float64
		}{
			{"3.14159265", 3.14159265},
			{"0.123123", 0.123123},
			{"-123456.789", -123456.789},
			{"-1.", -1.0},
		}
		for _, tc := range validCases {
			value, err := p.Parse(tc.in)
			assert.Equal(t, tc.expected, cast[float64](t, value), "should parse %q", tc.in)
			assert.NoError(t, err, "should parse %q without error", tc.in)
		}

		invalidCases := []string{"", "1.1.1", "1-2.0", "80-"}
		for _, in := range invalidCases {
			_, err := p.Parse(in)
			assert.Error(t, err, "should reject %q as a double", in)
		}
	})

	t.Run("using fieldint32parser", func(t *testing.T) {
		p, _ := NewFieldParser(ctInt32, "")

		validCases := []struct {
			in       string
			expected int32
			// the original test didn't check err on this boundary case
			checkErr bool
		}{
			{"2147483647", 2147483647, true},
			{"42", 42, true},
			{"-2147483648", -2147483648, false},
		}
		for _, tc := range validCases {
			value, err := p.Parse(tc.in)
			assert.Equal(t, tc.expected, cast[int32](t, value), "should parse %q", tc.in)
			if tc.checkErr {
				assert.NoError(t, err, "should parse %q without error", tc.in)
			}
		}

		invalidCases := []string{"", "42.0", "1-2", "80-", "2147483648", "-2147483649"}
		for _, in := range invalidCases {
			_, err := p.Parse(in)
			assert.Error(t, err, "should reject %q as an int32", in)
		}
	})

	t.Run("using fieldint64parser", func(t *testing.T) {
		p, _ := NewFieldParser(ctInt64, "")

		validCases := []struct {
			in       string
			expected int64
			// the original test didn't check err on this boundary case
			checkErr bool
		}{
			{"2147483648", 2147483648, true},
			{"42", 42, true},
			{"-2147483649", -2147483649, false},
		}
		for _, tc := range validCases {
			value, err := p.Parse(tc.in)
			assert.Equal(t, tc.expected, cast[int64](t, value), "should parse %q", tc.in)
			if tc.checkErr {
				assert.NoError(t, err, "should parse %q without error", tc.in)
			}
		}

		invalidCases := []string{"", "42.0", "1-2", "80-"}
		for _, in := range invalidCases {
			_, err := p.Parse(in)
			assert.Error(t, err, "should reject %q as an int64", in)
		}
	})

	t.Run("using fielddecimalparser", func(t *testing.T) {
		p, _ := NewFieldParser(ctDecimal, "")

		validCases := []string{"12235.2355", "42", "0", "-124", "-124.55"}
		for _, in := range validCases {
			want, err := bson.ParseDecimal128(in)
			require.NoError(t, err, "should parse %q as a decimal literal", in)
			value, err := p.Parse(in)
			assert.NoError(t, err, "should parse %q without error", in)
			assert.Equal(t, want, cast[bson.Decimal128](t, value), "should parse %q", in)
		}

		invalidCases := []string{"", "1-2", "abcd"}
		for _, in := range invalidCases {
			_, err := p.Parse(in)
			assert.Error(t, err, "should reject %q as a decimal", in)
		}
	})

	t.Run("using fieldstringparser", func(t *testing.T) {
		p, _ := NewFieldParser(ctString, "")

		cases := []struct {
			in       string
			expected string
		}{
			{"42", "42"},
			{"true", "true"},
			{"", ""},
		}
		for _, tc := range cases {
			value, err := p.Parse(tc.in)
			assert.Equal(t, tc.expected, cast[string](t, value), "should parse %q", tc.in)
			assert.NoError(t, err, "should parse %q without error", tc.in)
		}
	})
}

// cast asserts that val holds a T and returns the unwrapped value; T is only
// known to the caller, so the type-assertion check can't be folded into a
// plain testify equality call the way the rest of this file's comparisons
// are.
func cast[T any](t *testing.T, val any) T {
	t.Helper()

	converted, ok := val.(T)
	require.True(t, ok, "should decode as %T", converted)

	return converted
}
