// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"fmt"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// BufferedBulkInserter implements a bufio.Writer-like design for queuing up
// documents and inserting them in bulk when the given doc limit (or max
// message size) is reached. Must be flushed at the end to ensure that all
// documents are written.
type BufferedBulkInserter struct {
	bulk            *mgo.Bulk
	collection      *mgo.Collection
	continueOnError bool
	docLimit        int
	byteCount       int
	docCount        int
	unordered       bool
}

// NewBufferedBulkInserter returns an initialized BufferedBulkInserter
// for writing.
func NewBufferedBulkInserter(collection *mgo.Collection, docLimit int,
	continueOnError bool) *BufferedBulkInserter {
	bb := &BufferedBulkInserter{
		collection:      collection,
		continueOnError: continueOnError,
		docLimit:        docLimit,
	}
	bb.resetBulk()
	return bb
}

func (bb *BufferedBulkInserter) Unordered() {
	bb.unordered = true
	bb.bulk.Unordered()
}

// throw away the old bulk and init a new one
func (bb *BufferedBulkInserter) resetBulk() {
	bb.bulk = bb.collection.Bulk()
	if bb.continueOnError || bb.unordered {
		bb.bulk.Unordered()
	}
	bb.byteCount = 0
	bb.docCount = 0
}

// Insert adds a document to the buffer for bulk insertion. If the buffer is
// full, the bulk insert is made, returning any error that occurs.
func (bb *BufferedBulkInserter) Insert(doc interface{}) error {
	rawBytes, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("bson encoding error: %v", err)
	}
	// flush if we are full
	if bb.docCount >= bb.docLimit || bb.byteCount+len(rawBytes) > MaxBSONSize {
		err = bb.Flush()
	}
	// buffer the document
	bb.docCount++
	bb.byteCount += len(rawBytes)
	bb.bulk.Insert(bson.Raw{Data: rawBytes})
	return err
}

// Upsert adds a document to the buffer for bulk upsertion. If the buffer is
// full, the bulk upsert is made, returning any error that occurs.
// Upsert queues up the provided pair of upserting instructions.
// The first element of each pair selects which documents must be
// updated, and the second element defines how to update it.
// Each pair matches exactly one document for updating at most.
func (bb *BufferedBulkInserter) Upsert(pair []interface{}) error {
	if len(pair)%2 != 0 {
		return fmt.Errorf("Bulk.Upsert requires an even number of parameters")
	}
	selector := pair[0]
	if selector == nil {
		selector = bson.D{}
	}
	document := pair[1]
	rawBytes, err := bson.Marshal(document)
	if err != nil {
		return fmt.Errorf("bson encoding error: %v", err)
	}
	rawBytesSelector, errSelector := bson.Marshal(selector)
	if errSelector != nil {
		return fmt.Errorf("bson encoding error: %v", errSelector)
	}
	totalRequestSize := len(rawBytes) + len(rawBytesSelector)
	// flush if we are full
	if bb.docCount >= bb.docLimit || bb.byteCount+totalRequestSize > MaxBSONSize {
		err = bb.Flush()
	}
	// buffer the document
	bb.docCount++
	bb.byteCount += totalRequestSize
	bb.bulk.Upsert(selector, bson.Raw{Data: rawBytes})
	return err
}

// Remove queues up the provided selectors for removing matching documents.
// Each selector will remove only a single matching document.
func (bb *BufferedBulkInserter) Remove(selector interface{}) error {
	if selector == nil {
		return fmt.Errorf("Remove received a nil selector")
	}
	rawBytesSelector, err := bson.Marshal(selector)
	if err != nil {
		return fmt.Errorf("bson encoding error: %v", err)
	}
	if bb.docCount >= bb.docLimit || bb.byteCount+len(rawBytesSelector) > MaxBSONSize {
		err = bb.Flush()
	}
	bb.docCount++
	bb.byteCount += len(rawBytesSelector)
	bb.bulk.Remove(selector)
	return err
}

// Flush writes all buffered documents in one bulk operation then resets the buffer.
func (bb *BufferedBulkInserter) Flush() error {
	if bb.docCount == 0 {
		return nil
	}
	defer bb.resetBulk()
	if _, err := bb.bulk.Run(); err != nil {
		return err
	}
	return nil
}
