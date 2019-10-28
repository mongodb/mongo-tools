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

// FieldInfo contains information about field names. It is used in validateFields.
type FieldInfo struct {
	position int
	field    string
	parts    []string
}

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

// setNestedDocumentValue takes a nested field - in the form "a.b.c" - its associated value,
// and a document. It then assigns that value to the appropriate nested field within
// the document. If useArrayIndexFields is set to true, setNestedDocumentValue is mutually
// recursive with setNestedArrayValue. The two functions work together to set elements
// nested in documents and arrays. This is the strategy of setNestedDocumentValue/setNestedArrayValue:
//
// 1. setNestedDocumentValue is called first. The first part of the field is treated as
//    a document key, even if it is numeric. For a case such as 0.a.b, 0 would be
//    interpreted as a document key (which would only happen at the top level of a
//    BSON document being imported).
//
// 2. If there is only one field part, the value will be set for the field in the document.
//
// 3. setNestedDocumentValue will call setNestedArrayValue if the next part of the
//    field is a natural number (which implies the value is an element of an array).
//    Otherwise, it will call itself. If a document or array already exists for the field,
//    a reference to that document or array will be passed to setNestedDocumentValue or
//    setNestedArrayValue respectively. If no value exists, a new document or array is
//    created, added to the document, and a reference is passed to those functions.
//
// 4. If setNestedArrayValue has been called, the first part of the field is an array index.
//    If there is only one field part, setNestedArrayValue will append the provided value to the
//    provided array. This is only if the size of the array is equal to the index (meaning
//    elements of the array must be added sequentially: 0, 1, 2,...).
//
// 5. setNestedArrayValue will call setNestedDocumentValue if the next part of the field is not a
//    natural number (which implies the value is a document). setNestedArrayValue will call
//    itself if the next part of the field is a natural number. If a document or array already
//    exists at that index in the array, a reference to that document or array will be passed
//    to setNestedDocumentValue or setNestedArrayValue respectively. If no value exists, a new document
//    or array is created, added to the array, and a reference is passed to those functions.
func setNestedDocumentValue(fieldParts []string, value interface{}, document *bson.D, useArrayIndexFields bool) (err error) {
	if len(fieldParts) == 1 {
		*document = append(*document, bson.E{Key: fieldParts[0], Value: value})
		return nil
	}

	if _, ok := isNatNum(fieldParts[1]); useArrayIndexFields && ok {
		// next part of the field refers to an array
		elem, err := bsonutil.FindValueByKey(fieldParts[0], document)
		if err != nil {
			// element doesn't already exist
			subArray := &bson.A{}
			err = setNestedArrayValue(fieldParts[1:], value, subArray)
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
			err = setNestedArrayValue(fieldParts[1:], value, subArray)
			if err != nil {
				return err
			}
		}
	} else {
		// next part of the field refers to a document
		elem, err := bsonutil.FindValueByKey(fieldParts[0], document)
		if err != nil { // element doesn't already exist
			subDocument := &bson.D{}
			err = setNestedDocumentValue(fieldParts[1:], value, subDocument, useArrayIndexFields)
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
			err = setNestedDocumentValue(fieldParts[1:], value, subDocument, useArrayIndexFields)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// setNestedArrayValue takes a nested field, its associated value, and an array.
// It then assigns that value to  the appropriate nested field within the array.
// setNestedArrayValue is mutually recursive with setNestedDocumentValue. The two functions
// work together to set elements nested in documents and arrays. See the documentation
// of setNestedDocumentValue for more information.
func setNestedArrayValue(fieldParts []string, value interface{}, array *bson.A) (err error) {

	// The first part of the field should be an index of an array
	idx, ok := isNatNum(fieldParts[0])
	if !ok {
		return fmt.Errorf("setNestedArrayValue expected an integer field, but instead received %s", fieldParts[0])
	}

	if len(fieldParts) == 1 {
		if idx != len(*array) {
			return fmt.Errorf("Trying to add value to array at index %d, but array is %d elements long. "+
				"Array indices in fields must start from 0 and increase sequentially", idx, len(*array))
		}

		*array = append(*array, value)
		return nil
	}

	if _, ok := isNatNum(fieldParts[1]); ok {
		// next part of the field refers to an array
		if idx < len(*array) {
			// the index already exists in array
			// check the element is an array
			subArray, ok := (*array)[idx].(*bson.A)
			if !ok {
				return fmt.Errorf("Expected array element to be a sub-array, "+
					"but element has already been set as a document or other value: %#v", (*array)[idx])
			}
			err = setNestedArrayValue(fieldParts[1:], value, subArray)
			if err != nil {
				return err
			}
		} else {
			// the element at idx doesn't exist yet
			subArray := &bson.A{}
			err = setNestedArrayValue(fieldParts[1:], value, subArray)
			if err != nil {
				return err
			}
			*array = append(*array, subArray)
		}
	} else {
		// next part of the field refers to a document
		if idx < len(*array) {
			// the index already exists in array
			// check the element is an document
			subDocument, ok := (*array)[idx].(*bson.D)
			if !ok {
				return fmt.Errorf("Expected array element to be a document, "+
					"but element has already been set as another value: %#v", (*array)[idx])
			}
			err = setNestedDocumentValue(fieldParts[1:], value, subDocument, true)
			if err != nil {
				return err
			}
		} else {
			// the element at idx doesn't exist yet
			subDocument := &bson.D{}
			err = setNestedDocumentValue(fieldParts[1:], value, subDocument, true)
			if err != nil {
				return err
			}
			*array = append(*array, subDocument)
		}
	}
	return nil
}

// isNatNum returns a number and true if the string can be parsed as a natural number (including 0)
// The first byte of the string must be a number from 1-9. So "001" would not be parsed.
// Neither would phone numbers such as "+15558675309"
func isNatNum(s string) (int, bool) {
	if len(s) > 1 && s[0] == byte('0') { // don't allow 0 prefixes
		return 0, false
	}
	if s[0] >= byte('0') && s[0] <= byte('9') { // filter out symbols like +
		if num, err := strconv.Atoi(s); err == nil && num >= 0 {
			return num, true
		}
	}
	return 0, false
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
			tomb:                  importTomb,
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
func tokensToBSON(colSpecs []ColumnSpec, tokens []string, numProcessed uint64, ignoreBlanks bool, useArrayIndexFields bool) (bson.D, error) {
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
			if len(colSpecs[index].NameParts) > 1 {
				err = setNestedDocumentValue(colSpecs[index].NameParts, parsedValue, &document, useArrayIndexFields)
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
// are invalid, returns nil otherwise. Fields are invalid in the following cases:
//
//     (1). A field contains an invalid series of characters
//     (2). Two fields are the same (e.g. a,a)
//     (3). One field implies there is a value, another implies there is a document (e.g. a,a.b)
//
// In the case that --useArrayIndexFields is set, fields are also invalid in the following cases:
//
//     (4). One field implies there is a value, another implies there is an array (e.g. a,a.0).
//     (5). One field implies that there is a document, another implies there is an array.
//          (e.g. a.b,a.0 or a.b.c,a.0.c)
//     (6). The indexes for an array don't start from 0 (e.g. a.1,a.2)
//     (7). Array indexes are out of order (e.g. a.1,a.0 or a.0.b,a.1,a.0.c)
//     (8). An array is missing an index (e.g. a.0,a.2)
func validateFields(inputFields []string, useArrayIndexFields bool) error {
	for _, field := range inputFields {

		// Here we check validity for case (1).
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
	}
	// This checks all other cases by building attempting to build a legal tree of fields.
	// If useArrayIndexFields is set, the tree contains arrays of subtrees.
	fieldTree := make(map[string]interface{})

	for _, field := range inputFields {
		fieldParts := strings.Split(field, ".")
		_, err := addFieldToTree(fieldParts, 0, fieldTree, useArrayIndexFields)
		if err != nil {
			return err
		}
	}
	return nil
}

// addFieldToTree is a recursive function that builds up a tree of fields. It is used to check
// that fields are compatible with each other. When useArrayIndexFields is set, it is mutually recursive
// with addFieldToArray(). It closely mimics the behaviour of setNestedDocumentValue(). See validateFields()
// for more information on the validity checks that are made when constructing a tree of fields.
func addFieldToTree(fieldParts []string, partIndex int, tree map[string]interface{}, useArrayIndexFields bool) (map[string]interface{}, error) {
	value, exists := tree[fieldParts[partIndex]]

	if exists {
		if len(fieldParts[partIndex:]) == 1 {
			// fields are the same - case (2)
			return nil, fmt.Errorf("fields cannot be identical: '%v' and '%v'", strings.Join(fieldParts, "."), strings.Join(fieldParts, "."))
		} else if value == true {
			// case (3)
			return nil, fmt.Errorf("fields '%v' and '%v' are incompatible",
				strings.Join(fieldParts, "."),
				strings.Join(fieldParts[:partIndex+1], "."))
		}
	}

	if len(fieldParts[partIndex:]) == 1 {
		tree[fieldParts[partIndex]] = true
		return tree, nil
	}

	// Now determine the type of the next part of the field
	partIndex++
	if _, ok := isNatNum(fieldParts[partIndex]); useArrayIndexFields && ok {
		var subArray []interface{}
		if exists {
			subArray, ok = value.([]interface{})
			if !ok {
				// case (5) or (4)
				return nil, fmt.Errorf("fields '%v' and '%v' are incompatible",
					strings.Join(fieldParts[:partIndex], ".")+findFirstField(value),
					strings.Join(fieldParts, "."))
			}
		} else {
			subArray = make([]interface{}, 0)
		}
		subArray, err := addFieldToArray(fieldParts, partIndex, subArray)
		if err != nil {
			return nil, err
		}
		tree[fieldParts[0]] = subArray
	} else {
		var subTree map[string]interface{}
		if exists {
			subTree, ok = value.(map[string]interface{})
			if !ok {
				// case (5) or (4)
				return nil, fmt.Errorf("fields '%v' and '%v' are incompatible",
					strings.Join(fieldParts[:partIndex], ".")+findFirstField(value),
					strings.Join(fieldParts, "."))
			}
		} else {
			subTree = make(map[string]interface{})
		}
		subTree, err := addFieldToTree(fieldParts, partIndex, subTree, useArrayIndexFields)
		if err != nil {
			return nil, err
		}
		tree[fieldParts[0]] = subTree
	}
	return tree, nil

}

// addFieldToArray is used with addFieldToTree() to build a valid tree of fields.
func addFieldToArray(fieldParts []string, partIndex int, array []interface{}) ([]interface{}, error) {
	// The first part of the field should be an index of an array
	idx, ok := isNatNum(fieldParts[partIndex])
	if !ok {
		// We shouldn't ever get here
		return nil, fmt.Errorf("addFieldToArray expected an integer field, but instead received %s", fieldParts[0])
	}

	if len(fieldParts[partIndex:]) == 1 {
		// We're at the terminus of a field so we have to check if we can append to the array
		if idx < len(array) {
			// case (2)
			return nil, fmt.Errorf("fields cannot be identical: '%v' and '%v'",
				strings.Join(fieldParts, "."), strings.Join(fieldParts, "."))
		} else if idx > len(array) {

			if len(array) == 0 {
				// case (6) or (7)
				return nil, fmt.Errorf("array indexes must start from 0. No index found before index '%v' in field '%v'",
					idx, strings.Join(fieldParts, "."))
			}
			// case (7) and (8)
			return nil, fmt.Errorf("field '%v' contains an array index that does not increase sequentially from field '%v.%d'. "+
				"Array indexes in fields must start from 0 and increase sequentially",
				strings.Join(fieldParts, "."),
				strings.Join(fieldParts[:partIndex], "."),
				(len(array) - 1))
		}

		array = append(array, true)
		return array, nil
	}

	partIndex++
	if _, ok := isNatNum(fieldParts[partIndex]); ok {
		// next part of the field refers to an array
		if idx < len(array) {
			// the index already exists in array
			// check the element is an array
			subArray, ok := array[idx].([]interface{})
			if !ok {
				return nil, fmt.Errorf("fields '%v' and '%v' are incompatible",
					strings.Join(fieldParts[:partIndex], ".")+findFirstField((array)[idx]),
					strings.Join(fieldParts, "."))
			}
			subArray, err := addFieldToArray(fieldParts, partIndex, subArray)
			if err != nil {
				return nil, err
			}
			array[idx] = subArray
		} else {
			// the element at idx doesn't exist yet
			subArray := make([]interface{}, 0)
			subArray, err := addFieldToArray(fieldParts, partIndex, subArray)
			if err != nil {
				return nil, err
			}
			array = append(array, subArray)
		}
	} else {
		// next part of the field refers to a document
		if idx < len(array) {
			// the index already exists in array
			// check the element is an document
			subTree, ok := array[idx].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("fields '%v' and '%v' are incompatible",
					strings.Join(fieldParts[:partIndex], ".")+findFirstField((array)[idx]),
					strings.Join(fieldParts, "."))
			}
			subTree, err := addFieldToTree(fieldParts, partIndex, subTree, true)
			if err != nil {
				return nil, err
			}
			array[idx] = subTree
		} else {
			// the element at idx doesn't exist yet
			subTree := make(map[string]interface{})
			subTree, err := addFieldToTree(fieldParts, partIndex, subTree, true)
			if err != nil {
				return nil, err
			}
			array = append(array, subTree)
		}
	}
	return array, nil

}

// findFirstField is used as a helper for constructing error messages
// when building a tree of fields. It returns the path to the left-most leaf in a tree.
func findFirstField(i interface{}) string {
	switch v := i.(type) {
	case bool:
		return ""
	case []interface{}:
		return ".0" + findFirstField(v[0])
	case map[string]interface{}:
		for k, v := range v {
			return "." + k + findFirstField(v)
		}
	}
	return ""
}

// validateReaderFields is a helper to validate fields for input readers
func validateReaderFields(fields []string, useArrayIndexFields bool) error {
	if err := validateFields(fields, useArrayIndexFields); err != nil {
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
