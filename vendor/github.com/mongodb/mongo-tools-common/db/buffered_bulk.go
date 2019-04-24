// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BufferedBulkInserter implements a bufio.Writer-like design for queuing up
// documents and inserting them in bulk when the given doc limit (or max
// message size) is reached. Must be flushed at the end to ensure that all
// documents are written.
type BufferedBulkInserter struct {
	collection    *mongo.Collection
	writeModels   []mongo.WriteModel
	docLimit      int
	docCount      int
	bulkWriteOpts *options.BulkWriteOptions
	upsert        bool
}

// NewBufferedBulkInserter returns an initialized BufferedBulkInserter
// for writing.
func NewBufferedBulkInserter(collection *mongo.Collection, docLimit int, unordered bool) *BufferedBulkInserter {
	bb := &BufferedBulkInserter{
		collection:    collection,
		bulkWriteOpts: options.BulkWrite().SetOrdered(!unordered),
		docLimit:      docLimit,
		writeModels:   make([]mongo.WriteModel, 0, docLimit),
	}
	return bb
}

func (bb *BufferedBulkInserter) Unordered() {
	bb.bulkWriteOpts.SetOrdered(false)
}

func (bb *BufferedBulkInserter) SetBypassDocumentValidation(bypass bool) {
	bb.bulkWriteOpts.SetBypassDocumentValidation(bypass)
}

func (bb *BufferedBulkInserter) SetUpsert(upsert bool) {
	bb.upsert = upsert
}

// throw away the old bulk and init a new one
func (bb *BufferedBulkInserter) resetBulk() {
	bb.writeModels = bb.writeModels[:0]
	bb.docCount = 0
}

// Insert adds a document to the buffer for bulk insertion. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Insert(doc interface{}) error {
	rawBytes, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("bson encoding error: %v", err)
	}

	return bb.InsertRaw(rawBytes)
}

// Update adds a document to the buffer for bulk update. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Update(selector, update bson.D) error {
	return bb.addModel(mongo.NewUpdateOneModel().SetFilter(selector).SetUpdate(update).SetUpsert(bb.upsert))
}

// Replace adds a document to the buffer for bulk replacement. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Replace(selector, replacement bson.D) error {
	return bb.addModel(mongo.NewReplaceOneModel().SetFilter(selector).SetReplacement(replacement).SetUpsert(bb.upsert))
}

// InsertRaw adds a document, represented as raw bson bytes, to the buffer for bulk insertion. If the buffer becomes full,
// the bulk write is performed, returning any error that occurs.
func (bb *BufferedBulkInserter) InsertRaw(rawBytes []byte) error {
	return bb.addModel(mongo.NewInsertOneModel().SetDocument(rawBytes))
}

// addModel adds a WriteModel to the buffer. If the buffer becomes full, the bulk write is performed, returning any error
// that occurs.
func (bb *BufferedBulkInserter) addModel(model mongo.WriteModel) error {
	bb.docCount++
	bb.writeModels = append(bb.writeModels, model)

	if bb.docCount >= bb.docLimit {
		return bb.Flush()
	}
	return nil
}

// Flush writes all buffered documents in one bulk write and then resets the buffer.
func (bb *BufferedBulkInserter) Flush() error {
	if bb.docCount == 0 {
		return nil
	}

	defer bb.resetBulk()
	_, err := bb.collection.BulkWrite(context.Background(), bb.writeModels, bb.bulkWriteOpts)
	return err
}
