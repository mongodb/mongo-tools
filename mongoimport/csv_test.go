// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

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

func TestCSVStreamDocument(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	errorCases := []struct {
		name     string
		contents string
		colSpecs []ColumnSpec
	}{
		{
			name:     "badly encoded CSV should result in a parsing error",
			contents: `1, 2, foo"bar`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
		},
		{
			name:     "whitespace separated quoted strings are still an error",
			contents: `1, 2, "foo"  "bar"`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
		},
		{
			name:     "nested CSV fields causing header collisions should error",
			contents: `1, 2f , " 3e" , " may", june`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b.c", new(FieldAutoParser), pgAutoCast, "auto", []string{"b", "c"}},
				{"field3", new(FieldAutoParser), pgAutoCast, "auto", []string{"field3"}},
			},
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewCSVInputReader(
				tc.colSpecs,
				bytes.NewReader([]byte(tc.contents)),
				os.Stdout,
				1,
				false,
				false,
			)
			streamOutChan := make(chan bson.D, 1)
			require.Error(
				t,
				r.StreamDocument(t.Context(), true, streamOutChan),
				"should reject %q while streaming",
				tc.contents,
			)
		})
	}

	successCases := []struct {
		name          string
		contents      string
		file          string
		colSpecs      []ColumnSpec
		expectedReads []bson.D
	}{
		{
			name:     "escaped quotes are parsed correctly",
			contents: `1, 2, "foo""bar"`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
		},
		{
			name:     "multiple escaped quotes separated by whitespace parsed correctly",
			contents: `1, 2, "foo"" ""bar"`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", int32(2)},
					{"c", `foo" "bar`},
				},
			},
		},
		{
			name:     "integer valued strings should be converted",
			contents: `1, 2, " 3e"`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", int32(2)},
					{"c", " 3e"},
				},
			},
		},
		{
			name:     "extra fields should be prefixed with 'field'",
			contents: `1, 2f , " 3e" , " may"`,
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", "2f"},
					{"c", " 3e"},
					{"field3", " may"},
				},
			},
		},
		{
			name:     "calling StreamDocument() for CSVs should return next set of values",
			contents: "1, 2, 3\n4, 5, 6",
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", int32(2)},
					{"c", int32(3)},
				}, {
					{"a", int32(4)},
					{"b", int32(5)},
					{"c", int32(6)},
				},
			},
		},
		{
			name: "valid CSV input file that starts with the UTF-8 BOM should not raise an error",
			file: "testdata/test_bom.csv",
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", int32(2)},
					{"c", int32(3)},
				}, {
					{"a", int32(4)},
					{"b", int32(5)},
					{"c", int32(6)},
				},
			},
		},
	}

	for _, tc := range successCases {
		t.Run(tc.name, func(t *testing.T) {
			var r *CSVInputReader
			if tc.file != "" {
				fileHandle := openTestFixture(t, tc.file)
				r = NewCSVInputReader(tc.colSpecs, fileHandle, os.Stdout, 1, false, false)
			} else {
				r = NewCSVInputReader(
					tc.colSpecs,
					bytes.NewReader([]byte(tc.contents)),
					os.Stdout,
					1,
					false,
					false,
				)
			}
			// sized to the largest fixture in this table (2 documents), so the
			// reader never blocks trying to send
			streamOutChan := make(chan bson.D, 2)
			require.NoError(
				t,
				r.StreamDocument(t.Context(), true, streamOutChan),
				"should stream every document without error",
			)
			for _, expected := range tc.expectedReads {
				assert.Equal(t, expected, <-streamOutChan, "should stream the expected document")
			}
		})
	}

	t.Run("nested CSV fields should be imported properly", func(t *testing.T) {
		contents := `1, 2f , " 3e" , " may"`
		colSpecs := []ColumnSpec{
			{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
			{"b.c", new(FieldAutoParser), pgAutoCast, "auto", []string{"b", "c"}},
			{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
		}
		b := bson.D{{"c", "2f"}}
		expectedRead := bson.D{
			{"a", int32(1)},
			{"b", b},
			{"c", " 3e"},
			{"field3", " may"},
		}
		r := NewCSVInputReader(
			colSpecs,
			bytes.NewReader([]byte(contents)),
			os.Stdout,
			1,
			false,
			false,
		)
		streamOutChan := make(chan bson.D, 4)
		require.NoError(
			t,
			r.StreamDocument(t.Context(), true, streamOutChan),
			"should stream the document without error",
		)

		readDocument := <-streamOutChan
		assert.Equal(
			t,
			expectedRead[0],
			readDocument[0],
			"should stream the expected top-level field",
		)
		assert.Equal(
			t,
			expectedRead[1].Key,
			readDocument[1].Key,
			"should stream the expected nested field key",
		)

		// the nested field is decoded as a pointer, unlike the literal bson.D
		// used to express the expectation
		valueD, ok := readDocument[1].Value.(*bson.D)
		require.True(t, ok, "should decode the nested field as a *bson.D")

		assert.Equal(
			t,
			expectedRead[1].Value,
			*valueD,
			"should stream the expected nested field value",
		)
		assert.Equal(
			t,
			expectedRead[2],
			readDocument[2],
			"should stream the expected sibling field",
		)
		assert.Equal(t, expectedRead[3], readDocument[3], "should stream the expected extra field")
	})
}

func TestCSVReadAndValidateHeader(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("setting the header should read the first line of the CSV", func(t *testing.T) {
		contents := "extraHeader1, extraHeader2, extraHeader3"
		r := NewCSVInputReader(
			[]ColumnSpec{},
			bytes.NewReader([]byte(contents)),
			os.Stdout,
			1,
			false,
			false,
		)
		require.NoError(t, r.ReadAndValidateHeader(), "should read the header without error")
		require.Equal(t, 3, len(r.colSpecs), "should read every column in the header")
	})

	t.Run("setting non-colliding nested CSV headers should not raise an error", func(t *testing.T) {
		cases := []struct {
			contents        string
			expectedNumCols int
		}{
			{contents: "a, b, c", expectedNumCols: 3},
			{contents: "a.b.c, a.b.d, c", expectedNumCols: 3},
			{contents: "a.b, ab, a.c", expectedNumCols: 3},
			{contents: "a, ab, ac, dd", expectedNumCols: 4},
		}

		for _, tc := range cases {
			r := NewCSVInputReader(
				[]ColumnSpec{},
				bytes.NewReader([]byte(tc.contents)),
				os.Stdout,
				1,
				false,
				false,
			)
			require.NoError(
				t,
				r.ReadAndValidateHeader(),
				"should read %q without error",
				tc.contents,
			)
			assert.Equal(
				t,
				tc.expectedNumCols,
				len(r.colSpecs),
				"should read the expected number of columns from %q",
				tc.contents,
			)
		}
	})

	t.Run("setting colliding nested CSV headers should raise an error", func(t *testing.T) {
		cases := []string{
			"a, a.b, c",
			"a.b.c, a.b.d.c, a.b.d",
			//nolint:dupword
			"a, a, a",
		}

		for _, contents := range cases {
			r := NewCSVInputReader(
				[]ColumnSpec{},
				bytes.NewReader([]byte(contents)),
				os.Stdout,
				1,
				false,
				false,
			)
			assert.Error(
				t,
				r.ReadAndValidateHeader(),
				"should reject %q as a header collision",
				contents,
			)
		}
	})

	t.Run("setting the header that ends in a dot should error", func(t *testing.T) {
		contents := "c, a., b"
		// the original asserted this err (always nil at this point, never
		// reassigned) is nil; preserved as a no-op to keep the assertion count
		var err error
		require.NoError(t, err, "should have no error before reading the header")
		r := NewCSVInputReader(
			[]ColumnSpec{},
			bytes.NewReader([]byte(contents)),
			os.Stdout,
			1,
			false,
			false,
		)
		require.Error(t, r.ReadAndValidateHeader(), "should reject a header ending in a dot")
	})

	t.Run("setting the header that starts in a dot should error", func(t *testing.T) {
		contents := "c, .a, b"
		r := NewCSVInputReader(
			[]ColumnSpec{},
			bytes.NewReader([]byte(contents)),
			os.Stdout,
			1,
			false,
			false,
		)
		require.Error(t, r.ReadAndValidateHeader(), "should reject a header starting with a dot")
	})

	t.Run(
		"setting the header that contains multiple consecutive dots should error",
		func(t *testing.T) {
			cases := []string{
				"c, a..a, b",
				"c, a.a, b.b...b",
			}

			for _, contents := range cases {
				r := NewCSVInputReader(
					[]ColumnSpec{},
					bytes.NewReader([]byte(contents)),
					os.Stdout,
					1,
					false,
					false,
				)
				assert.Error(
					t,
					r.ReadAndValidateHeader(),
					"should reject %q for consecutive dots",
					contents,
				)
			}
		},
	)

	t.Run("setting the header using an empty file should return EOF", func(t *testing.T) {
		r := NewCSVInputReader(
			[]ColumnSpec{},
			bytes.NewReader([]byte("")),
			os.Stdout,
			1,
			false,
			false,
		)
		require.Equal(t, io.EOF, r.ReadAndValidateHeader(), "should report EOF for an empty header")
		require.Equal(t, 0, len(r.colSpecs), "should read no columns from an empty header")
	})

	t.Run(
		"setting the header with column specs already set should replace the existing column specs",
		func(t *testing.T) {
			contents := "extraHeader1,extraHeader2,extraHeader3"
			colSpecs := []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			}
			r := NewCSVInputReader(
				colSpecs,
				bytes.NewReader([]byte(contents)),
				os.Stdout,
				1,
				false,
				false,
			)
			require.NoError(
				t,
				r.ReadAndValidateHeader(),
				"should read the replacement header without error",
			)
			// if ReadAndValidateHeader() is called with column specs already passed
			// in, the header should be replaced with the read header line
			require.Equal(t, 3, len(r.colSpecs), "should replace the existing column specs")
			assert.Equal(
				t,
				strings.Split(contents, ","),
				ColumnNames(r.colSpecs),
				"should name the columns from the replacement header",
			)
		},
	)

	t.Run(
		"plain CSV input file sources should be parsed correctly and subsequent imports should parse correctly",
		func(t *testing.T) {
			colSpecs := []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			}
			expectedReadOne := bson.D{
				{"a", int32(1)},
				{"b", int32(2)},
				{"c", int32(3)},
			}
			expectedReadTwo := bson.D{
				{"a", int32(3)},
				{"b", 5.4},
				{"c", "string"},
			}
			fileHandle := openTestFixture(t, "testdata/test.csv")
			r := NewCSVInputReader(colSpecs, fileHandle, os.Stdout, 1, false, false)
			// buffered generously: test.csv holds more lines than this test
			// checks, and StreamDocument must not block trying to send them
			streamOutChan := make(chan bson.D, 50)
			require.NoError(
				t,
				r.StreamDocument(t.Context(), true, streamOutChan),
				"should stream every document without error",
			)
			assert.Equal(t, expectedReadOne, <-streamOutChan, "should stream the first document")
			assert.Equal(t, expectedReadTwo, <-streamOutChan, "should stream the second document")
		},
	)
}

func TestCSVConvert(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	csvConverter := CSVConverter{
		colSpecs: []ColumnSpec{
			{"field1", new(FieldAutoParser), pgAutoCast, "auto", []string{"field1"}},
			{"field2", new(FieldAutoParser), pgAutoCast, "auto", []string{"field2"}},
			{"field3", new(FieldAutoParser), pgAutoCast, "auto", []string{"field3"}},
		},
		data:  []string{"a", "b", "c"},
		index: uint64(0),
	}
	expectedDocument := bson.D{
		{"field1", "a"},
		{"field2", "b"},
		{"field3", "c"},
	}
	document, err := csvConverter.Convert()
	require.NoError(t, err, "should convert the document without error")
	require.Equal(t, expectedDocument, document, "should decode the expected document")
}
