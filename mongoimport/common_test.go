// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/sync/errgroup"
)

func init() {
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

var (
	index         = uint64(0)
	csvConverters = []CSVConverter{
		{
			colSpecs: []ColumnSpec{
				{"field1", new(FieldAutoParser), pgAutoCast, "auto", []string{"field1"}},
				{"field2", new(FieldAutoParser), pgAutoCast, "auto", []string{"field2"}},
				{"field3", new(FieldAutoParser), pgAutoCast, "auto", []string{"field3"}},
			},
			data:  []string{"a", "b", "c"},
			index: index,
		},
		{
			colSpecs: []ColumnSpec{
				{"field4", new(FieldAutoParser), pgAutoCast, "auto", []string{"field4"}},
				{"field5", new(FieldAutoParser), pgAutoCast, "auto", []string{"field5"}},
				{"field6", new(FieldAutoParser), pgAutoCast, "auto", []string{"field6"}},
			},
			data:  []string{"d", "e", "f"},
			index: index,
		},
		{
			colSpecs: []ColumnSpec{
				{"field7", new(FieldAutoParser), pgAutoCast, "auto", []string{"field7"}},
				{"field8", new(FieldAutoParser), pgAutoCast, "auto", []string{"field8"}},
				{"field9", new(FieldAutoParser), pgAutoCast, "auto", []string{"field9"}},
			},
			data:  []string{"d", "e", "f"},
			index: index,
		},
		{
			colSpecs: []ColumnSpec{
				{"field10", new(FieldAutoParser), pgAutoCast, "auto", []string{"field10"}},
				{"field11", new(FieldAutoParser), pgAutoCast, "auto", []string{"field11"}},
				{"field12", new(FieldAutoParser), pgAutoCast, "auto", []string{"field12"}},
			},
			data:  []string{"d", "e", "f"},
			index: index,
		},
		{
			colSpecs: []ColumnSpec{
				{"field13", new(FieldAutoParser), pgAutoCast, "auto", []string{"field13"}},
				{"field14", new(FieldAutoParser), pgAutoCast, "auto", []string{"field14"}},
				{"field15", new(FieldAutoParser), pgAutoCast, "auto", []string{"field15"}},
			},
			data:  []string{"d", "e", "f"},
			index: index,
		},
	}
	expectedDocuments = []bson.D{
		{
			{"field1", "a"},
			{"field2", "b"},
			{"field3", "c"},
		}, {
			{"field4", "d"},
			{"field5", "e"},
			{"field6", "f"},
		}, {
			{"field7", "d"},
			{"field8", "e"},
			{"field9", "f"},
		}, {
			{"field10", "d"},
			{"field11", "e"},
			{"field12", "f"},
		}, {
			{"field13", "d"},
			{"field14", "e"},
			{"field15", "f"},
		},
	}
)

func TestValidateFields(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		name    string
		fields  []string
		wantErr bool
	}{
		{"a field containing '..'", []string{"a..a"}, true},
		{"a field starting in a '.'", []string{".a"}, true},
		{"a field ending in a '.'", []string{"a."}, true},
		{"a field starting with '$.'", []string{"$.a"}, true},
		{"a field that is just '$'", []string{"$"}, true},
		{"a field starting with '$'", []string{"$a"}, true},
		{"a field with '$' not in the leading position", []string{"a$a"}, false},
		{"fields colliding on a nested prefix", []string{"a", "a.a"}, true},
		{"fields colliding on a shared nested prefix", []string{"a", "a.ba", "b.a"}, true},
		{
			"fields colliding on the same shared nested prefix again",
			[]string{"a", "a.ba", "b.a"},
			true,
		},
		{"fields colliding several levels deep", []string{"a", "a.b.c"}, true},
		{"fields sharing only a common substring", []string{"a", "aa"}, false},
		{"several fields sharing only common substrings", []string{"a", "aa", "b.a", "b.c"}, false},
		{"unrelated top-level and nested fields", []string{"a", "ba", "ab", "b.a"}, false},
		{
			"unrelated top-level and deeply nested fields",
			[]string{"a", "ba", "ab", "b.a", "b.c.d"},
			false,
		},
		{"a top-level field and an unrelated nested field", []string{"a", "ab.c"}, false},
		{"the same field repeated", []string{"a", "ba", "a"}, true},
	}

	for _, tc := range cases {
		err := validateFields(tc.fields, false)
		if tc.wantErr {
			assert.Error(t, err, "should reject %s", tc.name)
		} else {
			assert.NoError(t, err, "should accept %s", tc.name)
		}
	}
}

func TestGetUpsertValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	inner := bson.D{{"b", 4}}
	cases := []struct {
		name     string
		doc      bson.D
		key      string
		expected any
	}{
		{"an unnested document", bson.D{{"a", 3}}, "a", 3},
		{"a nested document", bson.D{{"a", inner}}, "a.b", 4},
		{"a nested document pointer", bson.D{{"a", &inner}}, "a.b", 4},
		{"an unnested key that does not exist", bson.D{{"a", 4}}, "c", nil},
		{"a nested key that does not exist", bson.D{{"a", inner}}, "a.c", nil},
		{"a nested document pointer key that does not exist", bson.D{{"a", &inner}}, "a.c", nil},
		{"a nil document value", bson.D{{"a", nil}}, "a", nil},
	}

	for _, tc := range cases {
		actual := getUpsertValue(tc.key, tc.doc)
		if tc.expected == nil {
			assert.Nil(t, actual, "should return the value of the key for %s", tc.name)
		} else {
			assert.Equal(t, tc.expected, actual, "should return the value of the key for %s", tc.name)
		}
	}
}

func TestConstructUpsertDocument(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		name     string
		doc      bson.D
		fields   []string
		expected bson.D
	}{
		{
			name:     "a single field on an unnested document",
			doc:      bson.D{{"a", 3}},
			fields:   []string{"a"},
			expected: bson.D{{"a", 3}},
		},
		{
			name:     "one of several fields on an unnested document",
			doc:      bson.D{{"a", 3}, {"b", "string value"}},
			fields:   []string{"a"},
			expected: bson.D{{"a", 3}},
		},
		{
			name: "a nested field on a document with several fields",
			doc: bson.D{
				{"a", bson.D{{testCollection, 4}}},
				{"b", "string value"},
			},
			fields:   []string{"a.c"},
			expected: bson.D{{"a.c", 4}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upsertDocument := constructUpsertDocument(tc.fields, tc.doc)
			assert.Equal(t, tc.expected, upsertDocument, "should build the upsert document")
		})
	}

	t.Run("a key that does not exist in the document", func(t *testing.T) {
		doc := bson.D{{"a", 3}, {"b", "string value"}}
		upsertDocument := constructUpsertDocument([]string{testCollection}, doc)
		assert.Nil(t, upsertDocument, "should return no upsert document when the key is absent")
	})
}

func TestSetNestedDocumentValue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("top level fields are set and others unchanged", func(t *testing.T) {
		testDocument := newNestedValueTestDocument()
		expectedDocument := bson.E{"c", 4}

		err := setNestedDocumentValue([]string{"c"}, 4, testDocument, false)
		require.NoError(t, err, "should set the new top-level field")

		newDocument := *testDocument
		require.Equal(t, 3, len(newDocument), "should add exactly one field")
		assert.Equal(t, expectedDocument, newDocument[2], "should set the field to the given value")
	})

	t.Run("new nested top-level fields are set and others unchanged", func(t *testing.T) {
		testDocument := newNestedValueTestDocument()
		expectedDocument := bson.D{{"b", "4"}}

		err := setNestedDocumentValue([]string{"c", "b"}, "4", testDocument, false)
		require.NoError(t, err, "should set the new nested field")

		newDocument := *testDocument
		require.Equal(t, 3, len(newDocument), "should add exactly one top-level field")
		assert.Equal(t, "c", newDocument[2].Key, "should nest the new field under the given key")

		valMap, ok := newDocument[2].Value.(*bson.D)
		require.True(t, ok, "should store the nested field as a document pointer")

		assert.Equal(t, expectedDocument, *valMap, "should set the nested field to the given value")
	})

	t.Run("existing nested level fields are set and others unchanged", func(t *testing.T) {
		testDocument := newNestedValueTestDocument()
		expectedDocument := bson.D{{"c", "d"}, {"d", 9}}

		err := setNestedDocumentValue([]string{"b", "d"}, 9, testDocument, false)
		require.NoError(t, err, "should set the existing nested field")

		newDocument := *testDocument
		require.Equal(t, 2, len(newDocument), "should not add a new top-level field")
		assert.Equal(t, "b", newDocument[1].Key, "should update the existing nested key")

		valMap, ok := newDocument[1].Value.(*bson.D)
		require.True(t, ok, "should store the nested field as a document pointer")

		assert.Equal(
			t,
			expectedDocument,
			*valMap,
			"should add the new nested field alongside the existing one",
		)
	})

	t.Run("subsequent calls update fields accordingly", func(t *testing.T) {
		testDocument := newNestedValueTestDocument()
		expectedDocumentOne := bson.D{{"c", "d"}, {"d", 9}}
		expectedDocumentTwo := bson.E{"f", 23}

		err := setNestedDocumentValue([]string{"b", "d"}, 9, testDocument, false)
		require.NoError(t, err, "should set the existing nested field")

		newDocument := *testDocument
		require.Equal(t, 2, len(newDocument), "should not add a new top-level field")
		assert.Equal(t, "b", newDocument[1].Key, "should update the existing nested key")

		valDoc, ok := newDocument[1].Value.(*bson.D)
		require.True(t, ok, "should store the nested field as a document pointer")
		assert.Equal(
			t,
			expectedDocumentOne,
			*valDoc,
			"should add the new nested field alongside the existing one",
		)

		err = setNestedDocumentValue([]string{"f"}, 23, testDocument, false)
		require.NoError(t, err, "should set the second new top-level field")

		newDocument = *testDocument
		require.Equal(t, 3, len(newDocument), "should add exactly one more top-level field")
		assert.Equal(
			t,
			expectedDocumentTwo,
			newDocument[2],
			"should set the second field to its own value",
		)
	})
}

// GoConvey re-built this document fresh for every leaf it ran, since
// setNestedDocumentValue mutates it in place; each subtest needs the same
// fresh start.
func newNestedValueTestDocument() *bson.D {
	b := bson.D{{"c", "d"}}
	currentDocument := bson.D{
		{"a", 3},
		{"b", &b},
	}

	return &currentDocument
}

func TestRemoveBlankFields(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	unblankedDocument := bson.D{{"a", 3}, {"b", "hello"}}
	assert.Equal(
		t,
		unblankedDocument,
		removeBlankFields(unblankedDocument),
		"should return the same document unchanged when there are no blanks",
	)

	d := bson.D{
		{"a", ""},
		{"b", ""},
	}
	e := bson.D{
		{"a", ""},
		{"b", 1},
	}
	bsonDocument := bson.D{
		{"a", 0},
		{"b", ""},
		{"c", ""},
		{"d", &d},
		{"e", &e},
	}
	inner := bson.D{
		{"b", 1},
	}
	expectedDocument := bson.D{
		{"a", 0},
		{"e", inner},
	}
	assert.Equal(
		t,
		expectedDocument,
		removeBlankFields(bsonDocument),
		"should drop blank fields including nested ones",
	)
}

func TestTokensToBSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	colSpecs := []ColumnSpec{
		{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
		{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
		{"c", new(FieldAutoParser), pgAutoCast, "auto", []string{"c"}},
	}
	cases := []struct {
		name     string
		tokens   []string
		expected bson.D
	}{
		{
			name:   "the expected ordered BSON for the given column specs and tokens",
			tokens: []string{"1", "2", "hello"},
			expected: bson.D{
				{"a", int32(1)},
				{"b", int32(2)},
				{"c", "hello"},
			},
		},
		{
			name:   "additional tokens are prefixed with 'field' and an index",
			tokens: []string{"1", "2", "hello", "mongodb", "user"},
			expected: bson.D{
				{"a", int32(1)},
				{"b", int32(2)},
				{"c", "hello"},
				{"field3", "mongodb"},
				{"field4", "user"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bsonD, err := tokensToBSON(colSpecs, tc.tokens, uint64(0), false, false)
			require.NoError(t, err, "should convert the tokens without error")
			assert.Equal(t, tc.expected, bsonD, "should produce the expected BSON document")
		})
	}

	t.Run("an error is thrown if duplicate headers are found", func(t *testing.T) {
		colSpecs := []ColumnSpec{
			{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
			{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
			{"field3", new(FieldAutoParser), pgAutoCast, "auto", []string{"field3"}},
		}
		tokens := []string{"1", "2", "hello", "mongodb", "user"}
		_, err := tokensToBSON(colSpecs, tokens, uint64(0), false, false)
		require.Error(t, err, "should reject the duplicate headers")
	})

	t.Run("fields with nested values are set appropriately", func(t *testing.T) {
		colSpecs := []ColumnSpec{
			{"a", new(FieldAutoParser), pgAutoCast, "auto", []string{"a"}},
			{"b", new(FieldAutoParser), pgAutoCast, "auto", []string{"b"}},
			{"c.a", new(FieldAutoParser), pgAutoCast, "auto", []string{"c", "a"}},
		}
		tokens := []string{"1", "2", "hello"}
		c := bson.D{
			{"a", "hello"},
		}
		expectedDocument := bson.D{
			{"a", int32(1)},
			{"b", int32(2)},
			{"c", c},
		}
		bsonD, err := tokensToBSON(colSpecs, tokens, uint64(0), false, false)
		require.NoError(t, err, "should convert the tokens without error")
		for i := range 2 {
			assert.Equal(
				t,
				bsonD[i].Key,
				expectedDocument[i].Key,
				"should keep field %d's key unchanged",
				i,
			)
			assert.Equal(
				t,
				bsonD[i].Value,
				expectedDocument[i].Value,
				"should keep field %d's value unchanged",
				i,
			)
		}
		assert.Equal(
			t,
			bsonD[2].Key,
			expectedDocument[2].Key,
			"should keep the nested key unchanged",
		)

		valueD, ok := bsonD[2].Value.(*bson.D)
		require.True(t, ok, "should store the nested field as a document pointer")

		assert.Equal(t, *valueD, expectedDocument[2].Value, "should set the nested value")
	})
}

func TestProcessDocuments(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// read-only: subtests only ever send these converters into a channel
	index := uint64(0)
	csvConverters := []CSVConverter{
		{
			colSpecs: []ColumnSpec{
				{"field1", new(FieldAutoParser), pgAutoCast, "auto", []string{"field1"}},
				{"field2", new(FieldAutoParser), pgAutoCast, "auto", []string{"field2"}},
				{"field3", new(FieldAutoParser), pgAutoCast, "auto", []string{"field3"}},
			},
			data:  []string{"a", "b", "c"},
			index: index,
		},
		{
			colSpecs: []ColumnSpec{
				{"field4", new(FieldAutoParser), pgAutoCast, "auto", []string{"field4"}},
				{"field5", new(FieldAutoParser), pgAutoCast, "auto", []string{"field5"}},
				{"field6", new(FieldAutoParser), pgAutoCast, "auto", []string{"field6"}},
			},
			data:  []string{"d", "e", "f"},
			index: index,
		},
	}
	expectedDocuments := []bson.D{
		{
			{"field1", "a"},
			{"field2", "b"},
			{"field3", "c"},
		}, {
			{"field4", "d"},
			{"field5", "e"},
			{"field6", "f"},
		},
	}

	t.Run("closes the input channel when ordered is true", func(t *testing.T) {
		docsInChan := make(chan Converter, 100)
		streamOutChan := make(chan bson.D, 100)
		iw := &importWorker{
			unprocessedDataChan:   docsInChan,
			processedDocumentChan: streamOutChan,
		}
		docsInChan <- csvConverters[0]
		docsInChan <- csvConverters[1]
		close(docsInChan)

		require.NoError(
			t,
			iw.processDocuments(t.Context(), true),
			"should process every queued document",
		)

		doc1, open := <-streamOutChan
		assert.Equal(t, expectedDocuments[0], doc1, "should emit the first converted document")
		assert.True(t, open, "should keep the output channel open after the first read")
		doc2, open := <-streamOutChan
		assert.Equal(t, expectedDocuments[1], doc2, "should emit the second converted document")
		assert.True(t, open, "should keep the output channel open after the second read")
		_, open = <-streamOutChan
		assert.False(t, open, "should close the output channel once ordered processing finishes")
	})

	t.Run("leaves the input channel open when ordered is false", func(t *testing.T) {
		docsInChan := make(chan Converter, 100)
		streamOutChan := make(chan bson.D, 100)
		iw := &importWorker{
			unprocessedDataChan:   docsInChan,
			processedDocumentChan: streamOutChan,
		}
		docsInChan <- csvConverters[0]
		docsInChan <- csvConverters[1]
		close(docsInChan)

		require.NoError(
			t,
			iw.processDocuments(t.Context(), false),
			"should process every queued document",
		)

		doc1, open := <-streamOutChan
		assert.Equal(t, expectedDocuments[0], doc1, "should emit the first converted document")
		assert.True(t, open, "should keep the output channel open after the first read")
		doc2, open := <-streamOutChan
		assert.Equal(t, expectedDocuments[1], doc2, "should emit the second converted document")
		assert.True(t, open, "should keep the output channel open after the second read")

		// close would panic if unordered processing had already closed streamOutChan
		close(streamOutChan)
	})
}

func TestDoSequentialStreaming(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	docsInChan := make(chan Converter, 5)
	streamOutChan := make(chan bson.D, 5)
	workerInputChannel := []chan Converter{
		make(chan Converter),
		make(chan Converter),
	}
	workerOutputChannel := []chan bson.D{
		make(chan bson.D),
		make(chan bson.D),
	}
	importWorkers := []*importWorker{
		{
			unprocessedDataChan:   workerInputChannel[0],
			processedDocumentChan: workerOutputChannel[0],
		},
		{
			unprocessedDataChan:   workerInputChannel[1],
			processedDocumentChan: workerOutputChannel[1],
		},
	}

	eg, ctx := errgroup.WithContext(t.Context())

	// start goroutines to do sequential processing
	for _, iw := range importWorkers {
		eg.Go(
			func() error { return iw.processDocuments(ctx, true) },
		)
	}
	// feed in a bunch of documents
	for _, inputCSVDocument := range csvConverters {
		docsInChan <- inputCSVDocument
	}
	close(docsInChan)
	doSequentialStreaming(ctx, eg, importWorkers, docsInChan, streamOutChan)
	for _, document := range expectedDocuments {
		assert.Equal(
			t,
			document,
			<-streamOutChan,
			"should process and return documents from the input channel in sequence",
		)
	}
}

func TestStreamDocuments(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("the pipeline completes without error under normal circumstances", func(t *testing.T) {
		docsInChan := make(chan Converter, 5)
		streamOutChan := make(chan bson.D, 5)

		// stream in some documents
		for _, csvConverter := range csvConverters {
			docsInChan <- csvConverter
		}
		close(docsInChan)
		require.NoError(
			t,
			streamDocuments(t.Context(), true, 3, docsInChan, streamOutChan),
			"should stream every document without error",
		)

		// ensure documents are streamed out and processed in the correct manner
		for _, expectedDocument := range expectedDocuments {
			assert.Equal(
				t,
				expectedDocument,
				<-streamOutChan,
				"should stream out the documents in order",
			)
		}
	})

	t.Run("the pipeline completes with error if an error is encountered", func(t *testing.T) {
		docsInChan := make(chan Converter, 5)
		streamOutChan := make(chan bson.D, 5)

		// stream in some documents - create duplicate headers to simulate an error
		csvConverter := CSVConverter{
			colSpecs: []ColumnSpec{
				{"field1", new(FieldAutoParser), pgAutoCast, "auto", []string{"field1"}},
				{"field2", new(FieldAutoParser), pgAutoCast, "auto", []string{"field2"}},
			},
			data:  []string{"a", "b", "c"},
			index: uint64(0),
		}
		docsInChan <- csvConverter
		close(docsInChan)

		require.Error(
			t,
			streamDocuments(t.Context(), true, 3, docsInChan, streamOutChan),
			"should return an error on the error channel",
		)
	})
}
