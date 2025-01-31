// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"fmt"
	"strings"

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
	docs          []bson.D
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
	ordered bool,
) *BufferedBulkInserter {
	bb := &BufferedBulkInserter{
		collection:    collection,
		bulkWriteOpts: options.BulkWrite().SetOrdered(ordered),
		docLimit:      docLimit,
		// We set the byte limit to be slightly lower than maxMessageSizeBytes so it can fit in one OP_MSG.
		// This may not always be perfect, e.g. we don't count update selectors in byte totals, but it should
		// be good enough to keep memory consumption in check.
		byteLimit:   MAX_MESSAGE_SIZE_BYTES - 100,
		writeModels: make([]mongo.WriteModel, 0, docLimit),
	}
	return bb
}

// NewOrderedBufferedBulkInserter returns an initialized BufferedBulkInserter for performing ordered bulk writes.
func NewOrderedBufferedBulkInserter(
	collection *mongo.Collection,
	docLimit int,
) *BufferedBulkInserter {
	return newBufferedBulkInserter(collection, docLimit, true)
}

// NewOrderedBufferedBulkInserter returns an initialized BufferedBulkInserter for performing unordered bulk writes.
func NewUnorderedBufferedBulkInserter(
	collection *mongo.Collection,
	docLimit int,
) *BufferedBulkInserter {
	return newBufferedBulkInserter(collection, docLimit, false)
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
	bb.docs = bb.docs[:0]
}

// Insert adds a document to the buffer for bulk insertion. If the buffer becomes full, the bulk write is performed, returning
// any error that occurs.
func (bb *BufferedBulkInserter) Insert(doc bson.D) (*mongo.BulkWriteResult, error) {
	rawBytes, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("bson encoding error: %v", err)
	}

	bb.docs = append(bb.docs, doc)
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

	ctx := context.Background()  
	
	if bb.docCount == 0 {  
		return nil, nil  
	}  
	res, bulkWriteErr := bb.collection.BulkWrite(ctx, bb.writeModels, bb.bulkWriteOpts)  
	if bulkWriteErr == nil {  
		return res, nil  
	}  
	
	bulkWriteException, ok := bulkWriteErr.(mongo.BulkWriteException)  
	if !ok {
		return res, bulkWriteErr
	}
	
	var retryDocFilters []bson.D  
	
	for _, we := range bulkWriteException.WriteErrors {
		if we.Code == ErrDuplicateKeyCode {  
			var errDetails map[string]bson.Raw
			bson.Unmarshal(we.WriteError.Raw, &errDetails)
			var filter bson.D
			bson.Unmarshal(errDetails["keyValue"], &filter)

			exists, err := checkDocumentExistence(ctx, bb.collection, filter)  
			if err != nil {  
				return nil, err  
			}  
			if !exists {  
				retryDocFilters = append(retryDocFilters, filter)  
			}  else {
			}
		}  
	}  

	for _, filter := range retryDocFilters {
		for _, doc := range bb.docs {
			var exists bool
			var err error
			if compareDocumentWithKeys(filter, doc) {  
				for range(3) {  
					_, err = bb.collection.InsertOne(ctx, doc)  
					if err == nil {  
						break  
					}  
				}  
				exists, err = checkDocumentExistence(ctx, bb.collection, filter)  
				if err != nil {  
					return nil, err  
				}  
				if exists {
					break
				}  
			}  
			if !exists {  
				return nil, fmt.Errorf("could not insert document %+v", doc)  
			}  
		}  
	}  
	
	res.InsertedCount += int64(len(retryDocFilters))  
	return res, bulkWriteErr  
}


// extractValueByPath digs into a bson.D using a dotted path to retrieve the value
func extractValueByPath(doc bson.D, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = doc
	for _, part := range parts {
		switch curr := current.(type) {
		case bson.D:
			found := false
			for _, elem := range curr {
				if elem.Key == part {
					current = elem.Value
					found = true
					break
				}
			}
			if !found {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return current, true
}

// compareDocumentWithKeys checks if the key-value pairs in doc1 exist in doc2
func compareDocumentWithKeys(doc1 bson.D, doc2 bson.D) bool {
	for _, elem := range doc1 {
		value, exists := extractValueByPath(doc2, elem.Key)
		if !exists || value != elem.Value {
			return false
		}
	}
	return true
}

func checkDocumentExistence(ctx context.Context, collection *mongo.Collection, document bson.D) (bool, error) {
	findCmd := bson.D{
        {Key: "find", Value: collection.Name()},
        {Key: "filter", Value: document},
        {Key: "readConcern", Value: bson.D{{Key: "level", Value: "majority"}}},
    }

	db := collection.Database()

	var result bson.M
    err := db.RunCommand(ctx, findCmd).Decode(&result)
    if err != nil {
        return false, err
    }

	if cursor, ok := result["cursor"].(bson.M); ok {
		if firstBatch, ok := cursor["firstBatch"].(bson.A); ok && len(firstBatch) > 0 {
			return true, nil
		} else {
			return false, nil
		}
	} else {
		return false, err
	}

}