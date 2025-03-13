// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The default value of maxMessageSizeBytes
// See: https://docs.mongodb.com/manual/reference/command/hello/#mongodb-data-hello.maxMessageSizeBytes
const MAX_MESSAGE_SIZE_BYTES = 48000000

// BufferedBulkInserter implements a bufio.Writer-like design for queuing up
// documents and inserting them in bulk when the given doc limit (or max
// message size) is reached. Must be flushed at the end to ensure that all
// documents are written.
type BufferedBulkInserter struct {
	collection    *mongo.Collection
	writeModels   []mongo.WriteModel
	docLimit      int
	docCount      int
	byteCount     int
	byteLimit     int
	bulkWriteOpts *options.BulkWriteOptions
	upsert        bool
}

func newBufferedBulkInserter(
	collection *mongo.Collection,
	docLimit int,
	serverVersion Version,
	ordered bool,
) *BufferedBulkInserter {
	bulkOpts := options.BulkWrite().SetOrdered(ordered)

	if MongoCanAcceptLiteralZeroTimestamp(serverVersion) {
		bulkOpts.BypassEmptyTsReplacement = lo.ToPtr(true)
	}

	bb := &BufferedBulkInserter{
		collection:    collection,
		bulkWriteOpts: bulkOpts,
		docLimit:      docLimit,
		// We set the byte limit to be slightly lower than maxMessageSizeBytes so it can fit in one OP_MSG.
		// This may not always be perfect, e.g. we don't count update selectors in byte totals, but it should
		// be good enough to keep memory consumption in check.
		byteLimit:   MAX_MESSAGE_SIZE_BYTES - 100,
		writeModels: make([]mongo.WriteModel, 0, docLimit),
	}
	return bb
}

func (bb *BufferedBulkInserter) CanDoZeroTimestamp() bool {
	bypassSettingPtr := bb.bulkWriteOpts.BypassEmptyTsReplacement

	return bypassSettingPtr != nil && *bypassSettingPtr
}

// NewUnorderedBufferedBulkInserter returns an initialized BufferedBulkInserter for performing unordered bulk writes.
func NewUnorderedBufferedBulkInserter(
	collection *mongo.Collection,
	docLimit int,
	serverVersion Version,
) *BufferedBulkInserter {
	return newBufferedBulkInserter(collection, docLimit, serverVersion, false)
}

func (bb *BufferedBulkInserter) SetOrdered(ordered bool) *BufferedBulkInserter {
	bb.bulkWriteOpts.SetOrdered(ordered)
	return bb
}

func (bb *BufferedBulkInserter) SetBypassDocumentValidation(bypass bool) *BufferedBulkInserter {
	bb.bulkWriteOpts.SetBypassDocumentValidation(bypass)
	return bb
}

func (bb *BufferedBulkInserter) SetUpsert(upsert bool) *BufferedBulkInserter {
	bb.upsert = upsert
	return bb
}

// throw away the old bulk and init a new one.
func (bb *BufferedBulkInserter) ResetBulk() {
	bb.writeModels = bb.writeModels[:0]
	bb.docCount = 0
	bb.byteCount = 0
}

// Insert adds a document to the buffer for bulk insertion. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Insert(doc interface{}) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("bson encoding error: %v", err)
	}

	return bb.InsertRaw(rawBytes)
}

// Update adds a document to the buffer for bulk update. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Update(selector, update bson.D) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(update)
	if err != nil {
		return nil, err
	}
	bb.byteCount += len(rawBytes)

	return bb.addModel(
		mongo.NewUpdateOneModel().SetFilter(selector).SetUpdate(rawBytes).SetUpsert(bb.upsert),
	)
}

// Replace adds a document to the buffer for bulk replacement. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Replace(
	selector, replacement bson.D,
) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(replacement)
	if err != nil {
		return nil, err
	}
	bb.byteCount += len(rawBytes)

	return bb.addModel(
		mongo.NewReplaceOneModel().
			SetFilter(selector).
			SetReplacement(rawBytes).
			SetUpsert(bb.upsert),
	)
}

// InsertRaw adds a document, represented as raw bson bytes, to the buffer for bulk insertion. If the buffer becomes full,
// the bulk write is performed, returning any error that occurs.
func (bb *BufferedBulkInserter) InsertRaw(rawBytes []byte) (*mongo.BulkWriteResult, error) {
	bb.byteCount += len(rawBytes)

	return bb.addModel(mongo.NewInsertOneModel().SetDocument(rawBytes))
}

// Delete adds a document to the buffer for bulk removal. If the buffer becomes full, the bulk delete is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Delete(
	selector, replacement bson.D,
) (*mongo.BulkWriteResult, error) {
	return bb.addModel(mongo.NewDeleteOneModel().SetFilter(selector))
}

// addModel adds a WriteModel to the buffer. If the buffer becomes full, the bulk write is performed, returning any error
// that occurs.
func (bb *BufferedBulkInserter) addModel(model mongo.WriteModel) (*mongo.BulkWriteResult, error) {
	bb.docCount++
	bb.writeModels = append(bb.writeModels, model)

	if bb.docCount >= bb.docLimit || bb.byteCount >= bb.byteLimit {
		return bb.Flush()
	}

	return nil, nil
}

// Flush writes all buffered documents in one bulk write and then resets the buffer.
func (bb *BufferedBulkInserter) Flush() (*mongo.BulkWriteResult, error) {
	defer bb.ResetBulk()
	return bb.flush()
}

// TryFlush writes all buffered documents in one bulk write without resetting the buffer.
func (bb *BufferedBulkInserter) TryFlush() (*mongo.BulkWriteResult, error) {
	return bb.flush()
}

func (bb *BufferedBulkInserter) flush() (*mongo.BulkWriteResult, error) {
	if bb.docCount == 0 {
		return nil, nil
	}

	return bb.collection.BulkWrite(context.Background(), bb.writeModels, bb.bulkWriteOpts)
}
