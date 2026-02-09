// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/xoptions"
)

// The default value of maxMessageSizeBytes
// See: https://docs.mongodb.com/manual/reference/command/hello/#mongodb-data-hello.maxMessageSizeBytes
const MAX_MESSAGE_SIZE_BYTES = 48000000

// BufferedBulkInserter implements a bufio.Writer-like design for queuing up
// documents and inserting them in bulk when the given doc limit (or max
// message size) is reached. Must be flushed at the end to ensure that all
// documents are written.
type BufferedBulkInserter struct {
	collection         *mongo.Collection
	writeModels        []mongo.WriteModel
	docLimit           int
	docCount           int
	byteCount          int
	byteLimit          int
	bulkWriteOpts      *options.BulkWriteOptionsBuilder
	upsert             bool
	canDoZeroTimestamp bool
}

func newBufferedBulkInserter(
	collection *mongo.Collection,
	docLimit int,
	serverVersion Version,
	ordered bool,
) *BufferedBulkInserter {
	bulkOpts := options.BulkWrite().SetOrdered(ordered)
	var zeroTimestampOk bool

	if MongoCanAcceptLiteralZeroTimestamp(serverVersion) {
		zeroTimestampOk = true
		err := xoptions.SetInternalBulkWriteOptions(bulkOpts, "addCommandFields", bson.D{
			{"bypassEmptyTsReplacement", true},
		})

		// This can only error if the call is malformed, which means we should never hit this in
		// production, so it's ok to panic here.
		if err != nil {
			panic("SetInternalBulkWriteOptions failed: " + err.Error())
		}
	}

	bb := &BufferedBulkInserter{
		collection:    collection,
		bulkWriteOpts: bulkOpts,
		docLimit:      docLimit,
		// We set the byte limit to be slightly lower than maxMessageSizeBytes so it can fit in one OP_MSG.
		// This may not always be perfect, e.g. we don't count update selectors in byte totals, but it should
		// be good enough to keep memory consumption in check.
		byteLimit:          MAX_MESSAGE_SIZE_BYTES - 100,
		writeModels:        make([]mongo.WriteModel, 0, docLimit),
		canDoZeroTimestamp: zeroTimestampOk,
	}
	return bb
}

func (bb *BufferedBulkInserter) CanDoZeroTimestamp() bool {
	return bb.canDoZeroTimestamp
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
func (bb *BufferedBulkInserter) Insert(
	ctx context.Context,
	doc any,
) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("bson encoding error: %v", err)
	}

	return bb.InsertRaw(ctx, rawBytes)
}

// Update adds a document to the buffer for bulk update. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Update(
	ctx context.Context,
	selector bson.D,
	update bson.D,
) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(update)
	if err != nil {
		return nil, err
	}
	bb.byteCount += len(rawBytes)

	return bb.addModel(
		ctx,
		mongo.NewUpdateOneModel().SetFilter(selector).SetUpdate(rawBytes).SetUpsert(bb.upsert),
	)
}

// Replace adds a document to the buffer for bulk replacement. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Replace(
	ctx context.Context,
	selector, replacement bson.D,
) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(replacement)
	if err != nil {
		return nil, err
	}
	bb.byteCount += len(rawBytes)

	return bb.addModel(
		ctx,
		mongo.NewReplaceOneModel().
			SetFilter(selector).
			SetReplacement(rawBytes).
			SetUpsert(bb.upsert),
	)
}

// InsertRaw adds a document, represented as raw bson bytes, to the buffer for bulk insertion. If the buffer becomes full,
// the bulk write is performed, returning any error that occurs.
func (bb *BufferedBulkInserter) InsertRaw(
	ctx context.Context,
	rawBytes []byte,
) (*mongo.BulkWriteResult, error) {
	bb.byteCount += len(rawBytes)

	return bb.addModel(ctx, mongo.NewInsertOneModel().SetDocument(rawBytes))
}

// Delete adds a document to the buffer for bulk removal. If the buffer becomes full, the bulk delete is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Delete(
	ctx context.Context,
	selector, replacement bson.D,
) (*mongo.BulkWriteResult, error) {
	return bb.addModel(ctx, mongo.NewDeleteOneModel().SetFilter(selector))
}

// addModel adds a WriteModel to the buffer. If the buffer becomes full, the bulk write is performed, returning any error
// that occurs.
func (bb *BufferedBulkInserter) addModel(
	ctx context.Context,
	model mongo.WriteModel,
) (*mongo.BulkWriteResult, error) {
	bb.docCount++
	bb.writeModels = append(bb.writeModels, model)

	if bb.docCount >= bb.docLimit || bb.byteCount >= bb.byteLimit {
		return bb.Flush(ctx)
	}

	return nil, nil
}

// Flush writes all buffered documents in one bulk write and then resets the buffer.
func (bb *BufferedBulkInserter) Flush(ctx context.Context) (*mongo.BulkWriteResult, error) {
	defer bb.ResetBulk()
	return bb.flush(ctx)
}

// TryFlush writes all buffered documents in one bulk write without resetting the buffer.
func (bb *BufferedBulkInserter) TryFlush(ctx context.Context) (*mongo.BulkWriteResult, error) {
	return bb.flush(ctx)
}

func (bb *BufferedBulkInserter) flush(ctx context.Context) (*mongo.BulkWriteResult, error) {
	if bb.docCount == 0 {
		return nil, nil
	}

	return bb.collection.BulkWrite(ctx, bb.writeModels, bb.bulkWriteOpts)
}
