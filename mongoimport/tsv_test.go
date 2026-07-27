// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"bytes"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestTSVStreamDocument(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		name          string
		contents      string
		file          string
		colSpecs      []ColumnSpec
		expectedReads []bson.D
	}{
		{
			name:     "integer valued strings should be converted tsv1",
			contents: "1\t2\t3e\n",
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", int32(2)},
					{"c", "3e"},
				},
			},
		},
		{
			name: "valid TSV input file that starts with the UTF-8 BOM should not raise an error",
			file: "testdata/test_bom.tsv",
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
				},
			},
		},
		{
			name:     "integer valued strings should be converted tsv2",
			contents: "a\tb\t\"cccc,cccc\"\td\n",
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", "a"},
					{"b", "b"},
					{"c", `"cccc,cccc"`},
					{"field3", "d"},
				},
			},
		},
		{
			name:     "extra columns should be prefixed with 'field'",
			contents: "1\t2\t3e\t may\n",
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(1)},
					{"b", int32(2)},
					{"c", "3e"},
					{"field3", " may"},
				},
			},
		},
		{
			name:     "mixed values should be parsed correctly",
			contents: "12\t13.3\tInline\t14\n",
			colSpecs: []ColumnSpec{
				{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
				{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
				{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
				{"d", new(FieldAutoParser), pgAutoCast, "auto", []string{"d"}},
			},
			expectedReads: []bson.D{
				{
					{"a", int32(12)},
					{"b", 13.3},
					{"c", "Inline"},
					{"d", int32(14)},
				},
			},
		},
		{
			name:     "calling StreamDocument() in succession for TSVs should return the correct next set of values",
			contents: "1\t2\t3\n4\t5\t6\n",
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
			name:     "calling StreamDocument() in succession for TSVs that contain quotes should return the correct next set of values",
			contents: "1\t2\t3\n4\t\"\t6\n",
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
				},
				{
					{"a", int32(4)},
					{"b", `"`},
					{"c", int32(6)},
				},
			},
		},
		{
			name: "plain TSV input file sources should be parsed correctly and subsequent imports should parse correctly",
			file: "testdata/test.tsv",
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
				},
				{
					{"a", int32(3)},
					{"b", 4.6},
					{"c", int32(5)},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r *TSVInputReader
			if tc.file != "" {
				fileHandle := openTestTSVFile(t, tc.file)
				r = NewTSVInputReader(tc.colSpecs, fileHandle, os.Stdout, 1, false, false)
			} else {
				r = NewTSVInputReader(
					tc.colSpecs,
					bytes.NewReader([]byte(tc.contents)),
					os.Stdout,
					1,
					false,
					false,
				)
			}
			// buffered generously: some fixtures contain more lines than this
			// test checks, and the reader must not block trying to send them
			streamOutChan := make(chan bson.D, 50)
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
}

// registers its own teardown so each table case gets a fresh handle.
func openTestTSVFile(t *testing.T, path string) *os.File {
	t.Helper()

	fileHandle, err := os.Open(path)
	require.NoError(t, err, "should open the test fixture")
	t.Cleanup(func() { fileHandle.Close() })

	return fileHandle
}

func TestTSVReadAndValidateHeader(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	contents := "extraHeader1\textraHeader2\textraHeader3\n"
	colSpecs := []ColumnSpec{}
	r := NewTSVInputReader(
		colSpecs,
		bytes.NewReader([]byte(contents)),
		os.Stdout,
		1,
		false,
		false,
	)
	require.NoError(t, r.ReadAndValidateHeader(), "should read the header without error")
	require.Equal(t, 3, len(r.colSpecs), "should read every column in the header")
}

func TestTSVConvert(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tsvConverter := TSVConverter{
		colSpecs: []ColumnSpec{
			{"field1", new(FieldAutoParser), pgAutoCast, "auto", []string{"field1"}},
			{"field2", new(FieldAutoParser), pgAutoCast, "auto", []string{"field2"}},
			{"field3", new(FieldAutoParser), pgAutoCast, "auto", []string{"field3"}},
		},
		data:  "a\tb\tc",
		index: uint64(0),
	}
	expectedDocument := bson.D{
		{"field1", "a"},
		{"field2", "b"},
		{"field3", "c"},
	}
	document, err := tsvConverter.Convert()
	require.NoError(t, err, "should convert the document without error")
	require.Equal(t, expectedDocument, document, "should decode the expected document")
}
