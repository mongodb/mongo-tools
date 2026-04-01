// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package importexport

import (
	"fmt"
	"os"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportDocumentValidation verifies mongoimport behavior with collection
// validators: normal import skips invalid docs, --bypassDocumentValidation
// imports all, --stopOnError and --maintainInsertionOrder fail on validation
// errors.
func (s *ImportExportSuite) TestImportDocumentValidation() {
	const dbName = "mongoimport_docvalidation_test"
	const collName = "docvalidation"

	client := s.newClient()

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
	_, err := db.Collection(collName).InsertMany(s.Context(), docs)
	s.Require().NoError(err)

	exportToolOptions, err := testutil.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	_, err = me.Export(tmpFile)
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	validator := bson.D{{"baz", bson.D{{"$exists", true}}}}
	ns := &options.Namespace{DB: dbName, Collection: collName}

	s.Run("no validator imports all docs", func() {
		s.Require().NoError(db.Drop(s.Context()))
		s.Require().NoError(s.importCollection(ns, tmpFile.Name(), mongoimport.IngestOptions{}))
		n, err := db.Collection(collName).CountDocuments(s.Context(), bson.D{})
		s.Require().NoError(err)
		s.Assert().
			EqualValues(1000, n, "import without validation should import all 1000 documents")
	})

	s.Run("validator skips invalid docs", func() {
		s.recreateWithValidator(db.Collection(collName), validator)
		s.Require().NoError(s.importCollection(ns, tmpFile.Name(), mongoimport.IngestOptions{}))
		n, err := db.Collection(collName).CountDocuments(s.Context(), bson.D{})
		s.Require().NoError(err)
		s.Assert().EqualValues(500, n, "only valid documents should be imported")
	})

	s.Run("stopOnError fails on validation errors", func() {
		s.recreateWithValidator(db.Collection(collName), validator)
		err := s.importCollection(ns, tmpFile.Name(), mongoimport.IngestOptions{StopOnError: true})
		s.assertValidationError(err, "import with --stopOnError should fail on validation errors")
	})

	s.Run("maintainInsertionOrder fails on validation errors", func() {
		s.recreateWithValidator(db.Collection(collName), validator)
		err := s.importCollection(
			ns, tmpFile.Name(), mongoimport.IngestOptions{MaintainInsertionOrder: true},
		)
		s.assertValidationError(
			err, "import with --maintainInsertionOrder should fail on validation errors",
		)
	})

	s.Run("bypassDocumentValidation imports all docs", func() {
		s.recreateWithValidator(db.Collection(collName), validator)
		s.Require().NoError(s.importCollection(
			ns, tmpFile.Name(), mongoimport.IngestOptions{BypassDocumentValidation: true},
		))
		n, err := db.Collection(collName).CountDocuments(s.Context(), bson.D{})
		s.Require().NoError(err)
		s.Assert().EqualValues(
			1000, n, "all documents should be imported with --bypassDocumentValidation",
		)
	})
}
