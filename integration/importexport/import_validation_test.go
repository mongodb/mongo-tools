// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package importexport

import (
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportDocumentValidation verifies mongoimport behavior with collection
// validators: normal import skips invalid docs, --bypassDocumentValidation
// imports all, --stopOnError and --maintainInsertionOrder fail on validation
// errors.
func TestImportDocumentValidation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_docvalidation_test"
	const collName = "docvalidation"

	client := newTestClient(t, dbName)

	db := client.Database(dbName)

	// 1000 docs: even-indexed lack `baz` (invalid), odd-indexed have `baz` (valid).
	docs := make([]any, 1000)
	for i := range 1000 {
		if i%2 == 0 {
			docs[i] = bson.D{{"_id", i}, {"num", i + 1}, {"s", fmt.Sprintf("%d", i)}}
		} else {
			docs[i] = bson.D{{"_id", i}, {"num", i + 1}, {"s", fmt.Sprintf("%d", i)}, {"baz", i}}
		}
	}
	_, err := db.Collection(collName).InsertMany(t.Context(), docs)
	require.NoError(t, err)

	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	require.NoError(t, err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	_, err = me.Export(tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	validator := bson.D{{"baz", bson.D{{"$exists", true}}}}
	ns := &options.Namespace{DB: dbName, Collection: collName}

	t.Run("no validator imports all docs", func(t *testing.T) {
		require.NoError(t, db.Drop(t.Context()))
		require.NoError(t, importCollection(t, ns, tmpFile.Name(), mongoimport.IngestOptions{}))
		n, err := db.Collection(collName).CountDocuments(t.Context(), bson.D{})
		require.NoError(t, err)
		assert.EqualValues(t, 1000, n, "import without validation should import all 1000 documents")
	})

	t.Run("validator skips invalid docs", func(t *testing.T) {
		recreateWithValidator(t, db.Collection(collName), validator)
		require.NoError(t, importCollection(t, ns, tmpFile.Name(), mongoimport.IngestOptions{}))
		n, err := db.Collection(collName).CountDocuments(t.Context(), bson.D{})
		require.NoError(t, err)
		assert.EqualValues(t, 500, n, "only valid documents should be imported")
	})

	t.Run("stopOnError fails on validation errors", func(t *testing.T) {
		recreateWithValidator(t, db.Collection(collName), validator)
		err := importCollection(t, ns, tmpFile.Name(), mongoimport.IngestOptions{StopOnError: true})
		assertValidationError(t, err, "import with --stopOnError should fail on validation errors")
	})

	t.Run("maintainInsertionOrder fails on validation errors", func(t *testing.T) {
		recreateWithValidator(t, db.Collection(collName), validator)
		err := importCollection(
			t, ns, tmpFile.Name(), mongoimport.IngestOptions{MaintainInsertionOrder: true},
		)
		assertValidationError(
			t, err, "import with --maintainInsertionOrder should fail on validation errors",
		)
	})

	t.Run("bypassDocumentValidation imports all docs", func(t *testing.T) {
		recreateWithValidator(t, db.Collection(collName), validator)
		require.NoError(t, importCollection(
			t, ns, tmpFile.Name(), mongoimport.IngestOptions{BypassDocumentValidation: true},
		))
		n, err := db.Collection(collName).CountDocuments(t.Context(), bson.D{})
		require.NoError(t, err)
		assert.EqualValues(
			t, 1000, n, "all documents should be imported with --bypassDocumentValidation",
		)
	})
}
