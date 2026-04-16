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

// MAX_MESSAGE_SIZE_BYTES default value of maxMessageSizeBytes
// See: https://docs.mongodb.com/manual/reference/command/hello/#mongodb-data-hello.maxMessageSizeBytes
const MAX_MESSAGE_SIZE_BYTES = 48000000

// MSG_OVERHEAD_BYTES Overhead budget for the OP_MSG wire message wrapping a bulk write.
// The Go driver v2 Batches.AppendBatchSequence checks only the cumulative
// document size against MaxMessageSize without subtracting the
// already-written command header, so we must reserve enough room for the
// wire message header, command body (insert command, $db, collection name,
// session, clusterTime, ordered, additionalCmd fields), and
// document-sequence framing.
const MSG_OVERHEAD_BYTES = 4 * 1024 * 1024

// BufferedBulkInserter implements a bufio.Writer-like design for queuing up
// documents and inserting them in bulk when the given doc limit (or max
// message size) is reached. Must be flushed at the end to ensure that all
// documents are written.
type BufferedBulkInserter struct {
	serverVersion      Version
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

	if serverVersion.IsEmpty() {
		panic("newBufferedBulkInserter requires non-empty server version")
	}

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

	if serverVersion.SupportsRawData() {
		err := xoptions.SetInternalBulkWriteOptions(bulkOpts, "rawData", true)
		if err != nil {
			panic("SetInternalBulkWriteOptions failed: " + err.Error())
		}
	}

	bb := &BufferedBulkInserter{
		serverVersion:      serverVersion,
		collection:         collection,
		bulkWriteOpts:      bulkOpts,
		docLimit:           docLimit,
		byteLimit:          MAX_MESSAGE_SIZE_BYTES - MSG_OVERHEAD_BYTES,
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

// Mongoimport needs to remove the rawData param because it explicitly operates on logical
// timeseries documents and not the underlying storage documents.
func (bb *BufferedBulkInserter) SetWithoutRawData() *BufferedBulkInserter {
	if !bb.serverVersion.SupportsRawData() {
		return bb
	}

	err := xoptions.SetInternalBulkWriteOptions(bb.bulkWriteOpts, "rawData", false)
	if err != nil {
		panic("SetInternalBulkWriteOptions failed: " + err.Error())
	}
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

	return bb.addModel(
		ctx,
		len(rawBytes),
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

	return bb.addModel(
		ctx,
		len(rawBytes),
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
	return bb.addModel(ctx, len(rawBytes), mongo.NewInsertOneModel().SetDocument(rawBytes))
}

// Delete adds a document to the buffer for bulk removal. If the buffer becomes full, the bulk delete is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Delete(
	ctx context.Context,
	selector, replacement bson.D,
) (*mongo.BulkWriteResult, error) {
	return bb.addModel(ctx, 0, mongo.NewDeleteOneModel().SetFilter(selector))
}

// addModel adds a WriteModel to the buffer. If adding the model would cause
// the buffer to exceed the byte limit, the current buffer is flushed first,
// ensuring we never send a wire message that exceeds maxMessageSizeBytes.
func (bb *BufferedBulkInserter) addModel(
	ctx context.Context,
	docSize int,
	model mongo.WriteModel,
) (*mongo.BulkWriteResult, error) {
	var (
		res *mongo.BulkWriteResult
		err error
	)

	if bb.docCount > 0 && bb.byteCount+docSize >= bb.byteLimit {
		res, err = bb.Flush(ctx)
		if err != nil {
			return res, err
		}
	}

	bb.docCount++
	bb.byteCount += docSize
	bb.writeModels = append(bb.writeModels, model)

	if bb.docCount >= bb.docLimit {
		return bb.Flush(ctx)
	}

	return res, err
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
