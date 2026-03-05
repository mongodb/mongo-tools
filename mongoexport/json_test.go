// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"bytes"
	"testing"

	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestWriteJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("ObjectId to extended JSON", func(t *testing.T) {
		out := new(bytes.Buffer)
		jsonExporter := NewJSONExportOutput(false, false, out, Relaxed)
		objId := bson.NewObjectID()
		err := jsonExporter.WriteHeader()
		require.NoError(t, err)
		err = jsonExporter.ExportDocument(bson.D{{"_id", objId}})
		require.NoError(t, err)
		err = jsonExporter.WriteFooter()
		require.NoError(t, err)
		assert.Equal(t, `{"_id":{"$oid":"`+objId.Hex()+`"}}`+"\n", out.String())
	})

	t.Run("canonical format", func(t *testing.T) {
		out := new(bytes.Buffer)
		exporter := NewJSONExportOutput(false, false, out, Canonical)

		err := exporter.WriteHeader()
		require.NoError(t, err)

		err = exporter.ExportDocument(bson.D{{"x", int32(1)}})
		require.NoError(t, err)

		err = exporter.WriteFooter()
		require.NoError(t, err)

		assert.Equal(t, `{"x":{"$numberInt":"1"}}`+"\n", out.String())
	})
}

func TestJSONArray(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	out := new(bytes.Buffer)

	jsonExporter := NewJSONExportOutput(true, false, out, Relaxed)
	err := jsonExporter.WriteHeader()
	require.NoError(t, err)

	// Export a few docs of various types
	testObjs := []any{
		bson.NewObjectID(),
		"asd",
		12345,
		3.14159,
		bson.D{{"A", 1}},
	}
	for _, obj := range testObjs {
		err := jsonExporter.ExportDocument(bson.D{{"_id", obj}})
		require.NoError(t, err)
	}

	err = jsonExporter.WriteFooter()
	require.NoError(t, err)

	// Unmarshal the whole thing, it should be valid json
	fromJSON := []map[string]any{}
	err = json.Unmarshal(out.Bytes(), &fromJSON)
	require.NoError(t, err)
	assert.Len(t, fromJSON, len(testObjs))
}
