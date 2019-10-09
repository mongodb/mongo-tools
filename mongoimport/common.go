// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mongodb/mongo-tools-common/bsonutil"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/bson"
	"gopkg.in/tomb.v2"
)

type ParseGrace int

const (
	pgAutoCast ParseGrace = iota
	pgSkipField
	pgSkipRow
	pgStop
)

// ValidatePG ensures the user-provided parseGrace is one of the allowed
// values.
func ValidatePG(pg string) (ParseGrace, error) {
	switch pg {
	case "autoCast":
		return pgAutoCast, nil
	case "skipField":
		return pgSkipField, nil
	case "skipRow":
		return pgSkipRow, nil
	case "stop":
		return pgStop, nil
	default:
		return pgAutoCast, fmt.Errorf("invalid parse grace: %s", pg)
	}
}

// ParsePG interprets the user-provided parseGrace, assuming it is valid.
func ParsePG(pg string) (res ParseGrace) {
	res, _ = ValidatePG(pg)
	return
}

// Converter is an interface that adds the basic Convert method which returns a
// valid BSON document that has been converted by the underlying implementation.
// If conversion fails, err will be set.
type Converter interface {
	Convert() (document bson.D, err error)
}

// An importWorker reads Converter from the unprocessedDataChan channel and
// sends processed BSON documents on the processedDocumentChan channel
type importWorker struct {
	// unprocessedDataChan is used to stream the input data for a worker to process
	unprocessedDataChan chan Converter

	// used to stream the processed document back to the caller
	processedDocumentChan chan bson.D

	// used to synchronise all worker goroutines
	tomb *tomb.Tomb
}

// an interface for tracking the number of bytes, which is used in mongoimport to feed
// the progress bar.
type sizeTracker interface {
	Size() int64
}

// sizeTrackingReader implements Reader and sizeTracker by wrapping an io.Reader and keeping track
// of the total number of bytes read from each call to Read().
type sizeTrackingReader struct {
	bytesRead int64
	reader    io.Reader
}

func (str *sizeTrackingReader) Size() int64 {
	bytes := atomic.LoadInt64(&str.bytesRead)
	return bytes
}

func (str *sizeTrackingReader) Read(p []byte) (n int, err error) {
	n, err = str.reader.Read(p)
	atomic.AddInt64(&str.bytesRead, int64(n))
	return
}

func newSizeTrackingReader(reader io.Reader) *sizeTrackingReader {
	return &sizeTrackingReader{
		reader:    reader,
		bytesRead: 0,
	}
}

var (
	UTF8_BOM = []byte{0xEF, 0xBB, 0xBF}
)

// bomDiscardingReader implements and wraps io.Reader, discarding the UTF-8 BOM, if applicable
type bomDiscardingReader struct {
	buf     *bufio.Reader
	didRead bool
}

func (bd *bomDiscardingReader) Read(p []byte) (int, error) {
	if !bd.didRead {
		bom, err := bd.buf.Peek(3)
		if err == nil && bytes.Equal(bom, UTF8_BOM) {
			bd.buf.Read(make([]byte, 3)) // discard BOM
		}
		bd.didRead = true
	}
	return bd.buf.Read(p)
}

func newBomDiscardingReader(r io.Reader) *bomDiscardingReader {
	return &bomDiscardingReader{buf: bufio.NewReader(r)}
}

// channelQuorumError takes a channel and a quorum - which specifies how many
// messages to receive on that channel before returning. It either returns the
// first non-nil error received on the channel or nil if up to `quorum` nil
// errors are received
func channelQuorumError(ch <-chan error, quorum int) (err error) {
	for i := 0; i < quorum; i++ {
		if err = <-ch; err != nil {
			return
		}
	}
	return
}

// constructUpsertDocument constructs a BSON document to use for upserts
func constructUpsertDocument(upsertFields []string, document bson.D) bson.D {
	upsertDocument := bson.D{}
	var hasDocumentKey bool
	for _, key := range upsertFields {
		val := getUpsertValue(key, document)
		if val != nil {
			hasDocumentKey = true
		}
		upsertDocument = append(upsertDocument, bson.E{Key: key, Value: val})
	}
	if !hasDocumentKey {
		return nil
	}
	return upsertDocument
}

// doSequentialStreaming takes a slice of workers, a readDocs (input) channel and
// an outputChan (output) channel. It sequentially writes unprocessed data read from
// the input channel to each worker and then sequentially reads the processed data
// from each worker before passing it on to the output channel
func doSequentialStreaming(workers []*importWorker, readDocs chan Converter, outputChan chan bson.D) {
	numWorkers := len(workers)

	// feed in the data to be processed and do round-robin
	// reads from each worker once processing is completed
	go func() {
		i := 0
		for doc := range readDocs {
			workers[i].unprocessedDataChan <- doc
			i = (i + 1) % numWorkers
		}

		// close the read channels of all the workers
		for i := 0; i < numWorkers; i++ {
			close(workers[i].unprocessedDataChan)
		}
	}()

	// coordinate the order in which the documents are sent over to the
	// main output channel
	numDoneWorkers := 0
	i := 0
	for {
		processedDocument, open := <-workers[i].processedDocumentChan
		if open {
			outputChan <- processedDocument
		} else {
			numDoneWorkers++
		}
		if numDoneWorkers == numWorkers {
			break
		}
		i = (i + 1) % numWorkers
	}
}

// getUpsertValue takes a given BSON document and a given field, and returns the
// field's associated value in the document. The field is specified using dot
// notation for nested fields. e.g. "person.age" would return 34 would return
// 34 in the document: bson.M{"person": bson.M{"age": 34}} whereas,
// "person.name" would return nil
func getUpsertValue(field string, document bson.D) interface{} {
	index := strings.Index(field, ".")
	if index == -1 {
		// grab the value (ignoring errors because we are okay with nil)
		val, _ := bsonutil.FindValueByKey(field, &document)
		return val
	}
	// recurse into subdocuments
	left := field[0:index]
	subDoc, _ := bsonutil.FindValueByKey(left, &document)
	if subDoc == nil {
		log.Logvf(log.DebugHigh, "no subdoc found for '%v'", left)
		return nil
	}
	switch subDoc.(type) {
	case bson.D:
		subDocD := subDoc.(bson.D)
		return getUpsertValue(field[index+1:], subDocD)
	case *bson.D:
		subDocD := subDoc.(*bson.D)
		return getUpsertValue(field[index+1:], *subDocD)
	default:
		log.Logvf(log.DebugHigh, "subdoc found for '%v', but couldn't coerce to bson.D", left)
		return nil
	}
}

// removeBlankFields takes document and returns a new copy in which
// fields with empty/blank values are removed
func removeBlankFields(document bson.D) (newDocument bson.D) {
	for _, keyVal := range document {
		if val, ok := keyVal.Value.(*bson.D); ok {
			keyVal.Value = removeBlankFields(*val)
		}
		if val, ok := keyVal.Value.(string); ok && val == "" {
			continue
		}
		if val, ok := keyVal.Value.(bson.D); ok && val == nil {
			continue
		}
		newDocument = append(newDocument, keyVal)
	}
	return newDocument
}

// setNestedValue takes a nested field - in the form "a.b.c" - its associated value,
// and a document. It then assigns that value to the appropriate nested field within
// the document. setNestedValue is mutually recursive with setNestedArrayValue. The
// two functions work together to set elements nested in documents and arrays.
// This is the strategy of setNestedValue/setNestedArrayValue:
//
// 1. setNestedValue is called first. The first part of the field is treated as
//    a document key, even if it is numeric. For a case such as 0.a.b, 0 would be
//    interpreted as a document key (which would only happen at the top level of a
//    BSON document being imported).
//
// 2. If there is only one field part, the value will be set for the field in the document.
//
// 3. setNestedValue will call setNestedArrayValue if the next part of the
//    field is a natural number (which implies the value is an element of an array).
//    Otherwise, it will call itself. If a document or array already exists for the field,
//    a reference to that document or array will be passed to setNestedValue or
//    setNestedArrayValue respectively. If no value exists, a new document or array is
//    created, added to the document, and a reference is passed to those functions.
//
// 4. If setNestedArrayValue has been called, the first part of the field is an array index.
//    If there is only one field part, setNestedArrayValue will append the provided value to the
//    provided array. This is only if the size of the array is equal to the index (meaning
//    elements of the array must be added sequentially: 0, 1, 2,...).
//
// 5. setNestedArrayValue will call setNestedValue if the next part of the
//    field is not a natural number (which implies the value is a document).
//    setNestedArrayValue will call itself if the next part of the field is a natural number.
//    If a document or array already exists at that index in the array, a reference to that
//    document or array will be passed to setNestedValue or setNestedArrayValue respectively.
//    If no value exists, a new document or array is created, added to the array, and a reference
//    is passed to those functions.
func setNestedValue(field string, value interface{}, document *bson.D) error {
	fieldParts := strings.Split(field, ".")

	if len(fieldParts) == 1 {
		*document = append(*document, bson.E{Key: fieldParts[0], Value: value})
		return nil
	}

	if isNatNum(fieldParts[1]) {
		// next part of the field refers to an array
		elem, err := bsonutil.FindValueByKey(fieldParts[0], document)
		if err != nil {
			// element doesn't already exist
			subArray := &bson.A{}
			err = setNestedArrayValue(strings.Join(fieldParts[1:], "."), value, subArray)
			if err != nil {
				return err
			}
			*document = append(*document, bson.E{Key: fieldParts[0], Value: subArray})
		} else {
			// element already exists
			// check the element is an array
			subArray, ok := elem.(*bson.A)
			if !ok {
				return fmt.Errorf("Expected document element to be an array, "+
					"but element has already been set as a document or other value: %#v", elem)
			}
			err = setNestedArrayValue(strings.Join(fieldParts[1:], "."), value, subArray)
			if err != nil {
				return err
			}
		}
	} else {
		// next part of the field refers to a document
		elem, err := bsonutil.FindValueByKey(fieldParts[0], document)
		if err != nil { // element doesn't already exist
			subDocument := &bson.D{}
			err = setNestedValue(strings.Join(fieldParts[1:], "."), value, subDocument)
			if err != nil {
				return err
			}
			*document = append(*document, bson.E{Key: fieldParts[0], Value: subDocument})
		} else {
			// element already exists
			// check the element is a document
			subDocument, ok := elem.(*bson.D)
			if !ok {
				return fmt.Errorf("Expected document element to be an document, "+
					"but element has already been set as another value: %#v", elem)
			}
			err = setNestedValue(strings.Join(fieldParts[1:], "."), value, subDocument)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// setNestedArrayValue takes a nested field, its associated value, and an array.
// It then assigns that value to  the appropriate nested field within the array.
// setNestedArrayValue is mutually recursive with setNestedValue. The two functions
// work together to set elements nested in documents and arrays. See the documentation
// of setNestedValue for more information.
func setNestedArrayValue(field string, value interface{}, array *bson.A) error {
	fieldParts := strings.Split(field, ".")

	// The first part of the field should be an index of an array
	idx, err := strconv.Atoi(fieldParts[0])
	if err != nil {
		return err
	}

	v := reflect.ValueOf(*array)

	if len(fieldParts) == 1 {
		if v.Len() != idx {
			return fmt.Errorf("Trying to add value to array at index %d, but array is %d elements long. "+
				"Array indices in fields must start from 0 and increase sequentially.", idx, v.Len())
		}

		*array = append(*array, value)
		return nil
	}

	if isNatNum(fieldParts[1]) {
		// next part of the field refers to an array
		if v.Len() > idx {
			// the index already exists in array
			// check the element is an array
			subArray, ok := (*array)[idx].(*bson.A)
			if !ok {
				return fmt.Errorf("Expected array element to be a sub-array, "+
					"but element has already been set as a document or other value: %#v", (*array)[idx])
			}
			err = setNestedArrayValue(strings.Join(fieldParts[1:], "."), value, subArray)
			if err != nil {
				return err
			}
		} else {
			// the element at idx doesn't exist yet
			subArray := &bson.A{}
			err = setNestedArrayValue(strings.Join(fieldParts[1:], "."), value, subArray)
			if err != nil {
				return err
			}
			*array = append(*array, subArray)
		}
	} else {
		// next part of the field refers to a document
		if v.Len() > idx {
			// the index already exists in array
			// check the element is an document
			subDocument, ok := (*array)[idx].(*bson.D)
			if !ok {
				return fmt.Errorf("Expected array element to be a document, "+
					"but element has already been set as another value: %#v", (*array)[idx])
			}
			err = setNestedValue(strings.Join(fieldParts[1:], "."), value, subDocument)
			if err != nil {
				return err
			}
		} else {
			// the element at idx doesn't exist yet
			subDocument := &bson.D{}
			err = setNestedValue(strings.Join(fieldParts[1:], "."), value, subDocument)
			if err != nil {
				return err
			}
			*array = append(*array, subDocument)
		}
	}
	return nil
}

// isNatNum returns true if the string can be parsed as a natural number (including 0)
func isNatNum(s string) bool {
	if num, err := strconv.Atoi(s); err == nil && num >= 0 {
		return true
	}
	return false
}

// streamDocuments concurrently processes data gotten from the inputChan
// channel in parallel and then sends over the processed data to the outputChan
// channel - either in sequence or concurrently (depending on the value of
// ordered) - in which the data was received
func streamDocuments(ordered bool, numDecoders int, readDocs chan Converter, outputChan chan bson.D) (retErr error) {
	if numDecoders == 0 {
		numDecoders = 1
	}
	var importWorkers []*importWorker
	wg := new(sync.WaitGroup)
	importTomb := new(tomb.Tomb)
	inChan := readDocs
	outChan := outputChan
	for i := 0; i < numDecoders; i++ {
		if ordered {
			inChan = make(chan Converter, workerBufferSize)
			outChan = make(chan bson.D, workerBufferSize)
		}
		iw := &importWorker{
			unprocessedDataChan:   inChan,
			processedDocumentChan: outChan,
			tomb: importTomb,
		}
		importWorkers = append(importWorkers, iw)
		wg.Add(1)
		go func(iw importWorker) {
			defer wg.Done()
			// only set the first worker error and cause sibling goroutines
			// to terminate immediately
			err := iw.processDocuments(ordered)
			if err != nil && retErr == nil {
				retErr = err
				iw.tomb.Kill(err)
			}
		}(*iw)
	}

	// if ordered, we have to coordinate the sequence in which processed
	// documents are passed to the main read channel
	if ordered {
		doSequentialStreaming(importWorkers, readDocs, outputChan)
	}
	wg.Wait()
	close(outputChan)
	return
}

// coercionError should only be used as a specific error type to check
// whether tokensToBSON wants the row to print
type coercionError struct{}

func (coercionError) Error() string { return "coercionError" }

// tokensToBSON reads in slice of records - along with ordered column names -
// and returns a BSON document for the record.
func tokensToBSON(colSpecs []ColumnSpec, tokens []string, numProcessed uint64, ignoreBlanks bool) (bson.D, error) {
	log.Logvf(log.DebugHigh, "got line: %v", tokens)
	var parsedValue interface{}
	document := bson.D{}
	for index, token := range tokens {
		if token == "" && ignoreBlanks {
			continue
		}
		if index < len(colSpecs) {
			parsedValue, err := colSpecs[index].Parser.Parse(token)
			if err != nil {
				log.Logvf(log.DebugHigh, "parse failure in document #%d for column '%s',"+
					"could not parse token '%s' to type %s",
					numProcessed, colSpecs[index].Name, token, colSpecs[index].TypeName)
				switch colSpecs[index].ParseGrace {
				case pgAutoCast:
					parsedValue = autoParse(token)
				case pgSkipField:
					continue
				case pgSkipRow:
					log.Logvf(log.Always, "skipping row #%d: %v", numProcessed, tokens)
					return nil, coercionError{}
				case pgStop:
					return nil, fmt.Errorf("type coercion failure in document #%d for column '%s', "+
						"could not parse token '%s' to type %s",
						numProcessed, colSpecs[index].Name, token, colSpecs[index].TypeName)
				}
			}
			if strings.Index(colSpecs[index].Name, ".") != -1 {
				// setNestedValue will set a subdocument to the key
				// 0
				err = setNestedValue(colSpecs[index].Name, parsedValue, &document)
				if err != nil {
					return nil, fmt.Errorf("can't set value for key %s: %s", colSpecs[index].Name, err)
				}
			} else {
				document = append(document, bson.E{Key: colSpecs[index].Name, Value: parsedValue})
			}
		} else {
			parsedValue = autoParse(token)
			key := "field" + strconv.Itoa(index)
			if util.StringSliceContains(ColumnNames(colSpecs), key) {
				return nil, fmt.Errorf("duplicate field name - on %v - for token #%v ('%v') in document #%v",
					key, index+1, parsedValue, numProcessed)
			}
			document = append(document, bson.E{Key: key, Value: parsedValue})
		}
	}
	return document, nil
}

// validateFields takes a slice of fields and returns an error if the fields
// are invalid, returns nil otherwise
func validateFields(fields []string) error {
	fieldsCopy := make([]string, len(fields), len(fields))
	copy(fieldsCopy, fields)
	sort.Sort(sort.StringSlice(fieldsCopy))

	for index, field := range fieldsCopy {
		if strings.HasSuffix(field, ".") {
			return fmt.Errorf("field '%v' cannot end with a '.'", field)
		}
		if strings.HasPrefix(field, ".") {
			return fmt.Errorf("field '%v' cannot start with a '.'", field)
		}
		if strings.HasPrefix(field, "$") {
			return fmt.Errorf("field '%v' cannot start with a '$'", field)
		}
		if strings.Contains(field, "..") {
			return fmt.Errorf("field '%v' cannot contain consecutive '.' characters", field)
		}
		// NOTE: since fields is sorted, this check ensures that no field
		// is incompatible with another one that occurs further down the list.
		// meant to prevent cases where we have fields like "a" and "a.c"

		// TODO: Add a check here for incompatible fields:
		// a and a.0
		// a.b and a.0
		for _, latterField := range fieldsCopy[index+1:] {
			// NOTE: this means we will not support imports that have fields that
			// include e.g. a, a.b
			if strings.HasPrefix(latterField, field+".") {
				return fmt.Errorf("fields '%v' and '%v' are incompatible", field, latterField)
			}
			// NOTE: this means we will not support imports that have fields like
			// a, a - since this is invalid in MongoDB
			if field == latterField {
				return fmt.Errorf("fields cannot be identical: '%v' and '%v'", field, latterField)
			}
		}
	}
	return nil
}

// validateReaderFields is a helper to validate fields for input readers
func validateReaderFields(fields []string) error {
	if err := validateFields(fields); err != nil {
		return err
	}
	if len(fields) == 1 {
		log.Logvf(log.Info, "using field: %v", fields[0])
	} else {
		log.Logvf(log.Info, "using fields: %v", strings.Join(fields, ","))
	}
	return nil
}

// processDocuments reads from the Converter channel and for each record, converts it
// to a bson.D document before sending it on the processedDocumentChan channel. Once the
// input channel is closed the processed channel is also closed if the worker streams its
// reads in order
func (iw *importWorker) processDocuments(ordered bool) error {
	if ordered {
		defer close(iw.processedDocumentChan)
	}
	for {
		select {
		case converter, alive := <-iw.unprocessedDataChan:
			if !alive {
				return nil
			}
			document, err := converter.Convert()
			if err != nil {
				return err
			}
			if document == nil {
				continue
			}
			iw.processedDocumentChan <- document
		case <-iw.tomb.Dying():
			return nil
		}
	}
}
