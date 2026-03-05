// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestWriteCSV(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	fields := []string{"_id", "x", " y", "z.1.a"}

	t.Run("with header", func(t *testing.T) {
		out := new(bytes.Buffer)
		csvExporter := NewCSVExportOutput(fields, false, out)
		err := csvExporter.WriteHeader()
		require.NoError(t, err)
		err = csvExporter.ExportDocument(bson.D{{"_id", "12345"}})
		require.NoError(t, err)
		err = csvExporter.WriteFooter()
		require.NoError(t, err)
		err = csvExporter.Flush()
		require.NoError(t, err)
		rec, err := csv.NewReader(strings.NewReader(out.String())).Read()
		require.NoError(t, err)
		assert.Equal(t, []string{"_id", "x", " y", "z.1.a"}, rec)
	})

	t.Run("without header", func(t *testing.T) {
		out := new(bytes.Buffer)
		csvExporter := NewCSVExportOutput(fields, true, out)
		err := csvExporter.WriteHeader()
		require.NoError(t, err)
		err = csvExporter.ExportDocument(bson.D{{"_id", "12345"}})
		require.NoError(t, err)
		err = csvExporter.WriteFooter()
		require.NoError(t, err)
		err = csvExporter.Flush()
		require.NoError(t, err)
		rec, err := csv.NewReader(strings.NewReader(out.String())).Read()
		require.NoError(t, err)
		assert.Equal(t, []string{"12345", "", "", ""}, rec)
	})

	t.Run("missing fields", func(t *testing.T) {
		out := new(bytes.Buffer)
		csvExporter := NewCSVExportOutput(fields, true, out)
		err := csvExporter.ExportDocument(bson.D{{"_id", "12345"}})
		require.NoError(t, err)
		err = csvExporter.WriteFooter()
		require.NoError(t, err)
		err = csvExporter.Flush()
		require.NoError(t, err)
		rec, err := csv.NewReader(strings.NewReader(out.String())).Read()
		require.NoError(t, err)
		assert.Equal(t, []string{"12345", "", "", ""}, rec)
	})

	t.Run("index into nested object", func(t *testing.T) {
		out := new(bytes.Buffer)
		csvExporter := NewCSVExportOutput(fields, true, out)
		z := []any{"x", bson.D{{"a", "T"}, {"B", 1}}}
		err := csvExporter.ExportDocument(bson.D{{Key: "z", Value: z}})
		require.NoError(t, err)
		err = csvExporter.WriteFooter()
		require.NoError(t, err)
		err = csvExporter.Flush()
		require.NoError(t, err)
		rec, err := csv.NewReader(strings.NewReader(out.String())).Read()
		require.NoError(t, err)
		assert.Equal(t, []string{"", "", "", "T"}, rec)
	})
}

func TestExtractDField(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	b := []any{"inner", bsonutil.MarshalD{{"inner2", 1}}}
	c := bsonutil.MarshalD{{"x", 5}}
	d := bsonutil.MarshalD{{"z", nil}}
	testD := bsonutil.MarshalD{
		{"a", "string"},
		{"b", b},
		{"c", c},
		{"d", d},
	}

	t.Run("regular fields", func(t *testing.T) {
		val := extractFieldByName("a", testD)
		assert.Equal(t, "string", val)
	})

	t.Run("array fields", func(t *testing.T) {
		val := extractFieldByName("b.1", testD)
		assert.Equal(t, bsonutil.MarshalD{{"inner2", 1}}, val)
		val = extractFieldByName("b.1.inner2", testD)
		assert.Equal(t, 1, val)
		val = extractFieldByName("b.0", testD)
		assert.Equal(t, "inner", val)
	})

	t.Run("subdocument fields", func(t *testing.T) {
		val := extractFieldByName("c", testD)
		assert.Equal(t, bsonutil.MarshalD{{"x", 5}}, val)
		val = extractFieldByName("c.x", testD)
		assert.Equal(t, 5, val)

		t.Run("with null values", func(t *testing.T) {
			val := extractFieldByName("d", testD)
			assert.Equal(t, bsonutil.MarshalD{{"z", nil}}, val)
			val = extractFieldByName("d.z", testD)
			assert.Equal(t, nil, val)
			val = extractFieldByName("d.z.nope", testD)
			assert.Empty(t, val)
		})
	})

	t.Run("non-existing fields", func(t *testing.T) {
		val := extractFieldByName("f", testD)
		assert.Empty(t, val)
		val = extractFieldByName("c.nope", testD)
		assert.Empty(t, val)
		val = extractFieldByName("c.nope.NOPE", testD)
		assert.Empty(t, val)
		val = extractFieldByName("b.1000", testD)
		assert.Empty(t, val)
		val = extractFieldByName("b.1.nada", testD)
		assert.Empty(t, val)
	})

	t.Run("non-document", func(t *testing.T) {
		val := extractFieldByName("meh", []any{"meh"})
		assert.Empty(t, val)
	})

	t.Run("nil document", func(t *testing.T) {
		val := extractFieldByName("a", nil)
		assert.Empty(t, val)
	})
}
