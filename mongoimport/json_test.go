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
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// For all tests that call NewJSONInputReader, the second parameter set to true indicates that we are testing legacy
// extended JSON instead of extended JSON v2.

func TestJSONArrayStreamDocument(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	errorCases := []struct {
		name     string
		contents string
	}{
		{
			name:     "an error should be thrown if a plain JSON document is supplied",
			contents: `{"a": "ae"}`,
		},
		{
			name:     "reading a JSON object that has no opening bracket should error out",
			contents: `{"a":3},{"b":4}]`,
		},
		{
			name:     "JSON arrays that do not end with a closing bracket should error out",
			contents: `[{"a": "ae"}`,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewJSONInputReader(true, true, bytes.NewReader([]byte(tc.contents)), 1)
			require.Error(
				t,
				r.StreamDocument(t.Context(), true, make(chan bson.D, 1)),
				"should error on invalid input",
			)
		})
	}

	t.Run("an error should be thrown if a plain JSON file is supplied", func(t *testing.T) {
		fileHandle := openTestJSONFile(t, "testdata/test_plain.json")
		r := NewJSONInputReader(true, true, fileHandle, 1)
		require.Error(
			t,
			r.StreamDocument(t.Context(), true, make(chan bson.D, 50)),
			"should error on a plain JSON file",
		)
	})

	t.Run(
		"array JSON input file sources should be parsed correctly and subsequent imports should parse correctly",
		func(t *testing.T) {
			// TODO: currently parses JSON as floats and not ints
			expectedReadOne := bson.D{
				{"a", 1.2},
				{"b", "a"},
				{"c", 0.4},
			}
			expectedReadTwo := bson.D{
				{"a", 2.4},
				{"b", "string"},
				{"c", 52.9},
			}
			fileHandle := openTestJSONFile(t, "testdata/test_array.json")
			r := NewJSONInputReader(true, true, fileHandle, 1)
			streamOutChan := make(chan bson.D, 50)
			require.NoError(
				t,
				r.StreamDocument(t.Context(), true, streamOutChan),
				"should stream both documents without error",
			)
			assert.Equal(t, expectedReadOne, <-streamOutChan, "should stream the first document")
			assert.Equal(t, expectedReadTwo, <-streamOutChan, "should stream the second document")
		},
	)
}

func TestJSONPlainStreamDocument(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		name          string
		contents      string
		expectedReads []bson.D
	}{
		{
			name:          "string valued JSON documents should be imported properly",
			contents:      `{"a": "ae"}`,
			expectedReads: []bson.D{{{"a", "ae"}}},
		},
		{
			name:     "several string valued JSON documents should be imported properly",
			contents: `{"a": "ae"}{"b": "dc"}`,
			expectedReads: []bson.D{
				{{"a", "ae"}},
				{{"b", "dc"}},
			},
		},
		{
			name:          "number valued JSON documents should be imported properly",
			contents:      `{"a": "ae", "b": 2.0}`,
			expectedReads: []bson.D{{{"a", "ae"}, {"b", 2.0}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewJSONInputReader(false, true, bytes.NewReader([]byte(tc.contents)), 1)
			streamOutChan := make(chan bson.D, len(tc.expectedReads))
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

	t.Run("JSON arrays should return an error", func(t *testing.T) {
		contents := `[{"a": "ae", "b": 2.0}]`
		r := NewJSONInputReader(false, true, bytes.NewReader([]byte(contents)), 1)
		require.Error(
			t,
			r.StreamDocument(t.Context(), true, make(chan bson.D, 50)),
			"should error on a JSON array",
		)
	})

	fileCases := []struct {
		name          string
		file          string
		expectedReads []bson.D
	}{
		{
			name: "plain JSON input file sources should be parsed correctly and subsequent imports should parse correctly",
			file: "testdata/test_plain.json",
			expectedReads: []bson.D{
				{
					{"a", 4},
					{"b", "string value"},
					{"c", 1},
				}, {
					{"a", 5},
					{"b", "string value"},
					{"c", 2},
				}, {
					{"a", 6},
					{"b", "string value"},
					{"c", 3},
				},
			},
		},
		{
			name: "reading JSON that starts with a UTF-8 BOM should not error",
			file: "testdata/test_bom.json",
			expectedReads: []bson.D{
				{
					{"a", 1},
					{"b", 2},
					{"c", 3},
				}, {
					{"a", 4},
					{"b", 5},
					{"c", 6},
				},
			},
		},
	}

	for _, tc := range fileCases {
		t.Run(tc.name, func(t *testing.T) {
			fileHandle := openTestJSONFile(t, tc.file)
			r := NewJSONInputReader(false, true, fileHandle, 1)
			streamOutChan := make(chan bson.D, len(tc.expectedReads))
			require.NoError(
				t,
				r.StreamDocument(t.Context(), true, streamOutChan),
				"should stream every document without error",
			)
			for _, expected := range tc.expectedReads {
				actual := <-streamOutChan
				for i, field := range actual {
					// the reader may decode numbers as a different numeric type than the
					// literal used in the expectation, so compare values rather than types
					assert.Equal(
						t,
						expected[i].Key,
						field.Key,
						"should stream the expected field key",
					)
					assert.EqualValues(
						t,
						expected[i].Value,
						field.Value,
						"should stream the expected field value",
					)
				}
			}
		})
	}
}

// registers its own teardown so each subtest gets a fresh handle.
func openTestJSONFile(t *testing.T, path string) *os.File {
	t.Helper()

	fileHandle, err := os.Open(path)
	require.NoError(t, err, "should open the test fixture")
	t.Cleanup(func() { fileHandle.Close() })

	return fileHandle
}

func TestReadJSONArraySeparator(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("reading a JSON array separator should consume [", func(t *testing.T) {
		contents := `[{"a": "ae"}`
		jsonImporter := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
		require.NoError(
			t,
			jsonImporter.readJSONArraySeparator(),
			"should consume the opening bracket",
		)
		// at this point it should have consumed all bytes up to `{`
		require.Error(
			t,
			jsonImporter.readJSONArraySeparator(),
			"should error with no more separators",
		)
	})

	t.Run(
		"reading a closing JSON array separator without a corresponding opening bracket should error out",
		func(t *testing.T) {
			contents := `]`
			jsonImporter := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			require.Error(
				t,
				jsonImporter.readJSONArraySeparator(),
				"should error without an opening bracket",
			)
		},
	)

	t.Run(
		"reading an opening JSON array separator without a corresponding closing bracket should error out",
		func(t *testing.T) {
			contents := `[`
			jsonImporter := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			require.NoError(
				t,
				jsonImporter.readJSONArraySeparator(),
				"should consume the opening bracket",
			)
			require.Error(
				t,
				jsonImporter.readJSONArraySeparator(),
				"should error without a closing bracket",
			)
		},
	)

	t.Run(
		"reading an opening JSON array separator with an ending closing bracket should return EOF",
		func(t *testing.T) {
			contents := `[]`
			jsonImporter := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			require.NoError(
				t,
				jsonImporter.readJSONArraySeparator(),
				"should consume the opening bracket",
			)
			require.Equal(
				t,
				io.EOF,
				jsonImporter.readJSONArraySeparator(),
				"should return EOF at the closing bracket",
			)
		},
	)

	t.Run(
		"reading an opening JSON array separator, an ending closing bracket but then additional characters after that, should error",
		func(t *testing.T) {
			contents := `[]a`
			jsonImporter := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			require.NoError(
				t,
				jsonImporter.readJSONArraySeparator(),
				"should consume the opening bracket",
			)
			require.Error(
				t,
				jsonImporter.readJSONArraySeparator(),
				"should error on trailing characters",
			)
		},
	)

	t.Run(
		"reading invalid JSON objects between valid objects should error out",
		func(t *testing.T) {
			contents := `[{"a":3}x{"b":4}]`
			r := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			streamOutChan := make(chan bson.D, 1)
			require.Error(
				t,
				r.StreamDocument(t.Context(), true, streamOutChan),
				"should error on the invalid document",
			)
			// read first valid document
			<-streamOutChan
			require.Error(t, r.readJSONArraySeparator(), "should error after the invalid document")
		},
	)

	t.Run(
		"reading invalid JSON objects after valid objects but between valid objects should error out",
		func(t *testing.T) {
			contents := `[{"a":3},b{"b":4}]`
			r := NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			require.Error(
				t,
				r.StreamDocument(t.Context(), true, make(chan bson.D, 1)),
				"should error on the invalid document",
			)

			contents = `[{"a":3},,{"b":4}]`
			r = NewJSONInputReader(true, true, bytes.NewReader([]byte(contents)), 1)
			require.Error(
				t,
				r.StreamDocument(t.Context(), true, make(chan bson.D, 1)),
				"should error on the empty element",
			)
		},
	)
}

func TestJSONConvert(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	data := []byte(`{"a": {"$binary": {"base64": "Zm9v", "subType": "03"}}}`)

	// $binary will be parsed as binary data in extended JSON v2 but not in legacy extended JSON
	extJSONDoc := bson.D{
		{"a", bson.Binary{
			Data:    []byte("foo"),
			Subtype: 3,
		}},
	}
	legacyJSONDoc := bson.D{
		{"a", bson.D{
			{"$binary", bson.D{
				{"base64", "Zm9v"},
				{"subType", "03"},
			}},
		}},
	}

	testCases := []struct {
		name          string
		data          []byte
		expectedDoc   bson.D
		legacyExtJSON bool
	}{
		{"new extended JSON", data, extJSONDoc, false},
		{"legacy extended JSON", data, legacyJSONDoc, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			converter := JSONConverter{
				data:          tc.data,
				legacyExtJSON: tc.legacyExtJSON,
			}

			doc, err := converter.Convert()
			require.NoError(t, err, "should convert the document without error")
			require.Equal(t, tc.expectedDoc, doc, "should decode the expected document")
		})
	}
}
