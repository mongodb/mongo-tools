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

// setNestedValue takes a nested field - in the form "a.b.c" - its associated value,
// and a document. It then assigns that value to the appropriate nested field within
// the document. If useArrayIndexFields is set to true, setNestedValue is mutually
// recursive with setNestedArrayValue. The two functions work together to set elements
// nested in documents and arrays. This is the strategy of setNestedValue/setNestedArrayValue:
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
// 5. setNestedArrayValue will call setNestedValue if the next part of the field is not a
//    natural number (which implies the value is a document). setNestedArrayValue will call
//    itself if the next part of the field is a natural number. If a document or array already
//    exists at that index in the array, a reference to that document or array will be passed
//    to setNestedValue or setNestedArrayValue respectively. If no value exists, a new document
//    or array is created, added to the array, and a reference is passed to those functions.
func setNestedValue(fieldParts []string, value interface{}, document *bson.D, useArrayIndexFields bool) error {
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
			err = setNestedValue(fieldParts[1:], value, subDocument, useArrayIndexFields)
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
			err = setNestedValue(fieldParts[1:], value, subDocument, useArrayIndexFields)
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
func setNestedArrayValue(fieldParts []string, value interface{}, array *bson.A) error {

	// The first part of the field should be an index of an array
	idx, err := strconv.Atoi(fieldParts[0])
	if err != nil {
		return fmt.Errorf("setNestedArrayValue expected an integer field, but instead received %s", fieldParts[0])
	}

	v := reflect.ValueOf(*array)

	if len(fieldParts) == 1 {
		if v.Len() != idx {
			return fmt.Errorf("Trying to add value to array at index %d, but array is %d elements long. "+
				"Array indices in fields must start from 0 and increase sequentially", idx, v.Len())
		}

		*array = append(*array, value)
		return nil
	}

	if _, ok := isNatNum(fieldParts[1]); ok {
		// next part of the field refers to an array
		if v.Len() > idx {
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
		if v.Len() > idx {
			// the index already exists in array
			// check the element is an document
			subDocument, ok := (*array)[idx].(*bson.D)
			if !ok {
				return fmt.Errorf("Expected array element to be a document, "+
					"but element has already been set as another value: %#v", (*array)[idx])
			}
			err = setNestedValue(fieldParts[1:], value, subDocument, true)
			if err != nil {
				return err
			}
		} else {
			// the element at idx doesn't exist yet
			subDocument := &bson.D{}
			err = setNestedValue(fieldParts[1:], value, subDocument, true)
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
				err = setNestedValue(colSpecs[index].NameParts, parsedValue, &document, useArrayIndexFields)
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
//     (4). One field implies there is a value, another implies there is an array (e.g. a,a.0). This
//          check is covered by the check for number 2.
//     (5). One field implies that there is a document, another implies there is an array.
//          (e.g. a.b,a.0 or a.b.c,a.0.c)
//     (6). The indexes for an array don't start from 0 (e.g. a.1,a.2)
//     (7). Array indexes are out of order (e.g. a.1,a.0 or a.0.b,a.1,a.0.c)
//     (8). An array is missing an index (e.g. a.0,a.2)
func validateFields(inputFields []string, useArrayIndexFields bool) error {
	fields := make([]FieldInfo, len(inputFields), len(inputFields))
	for i, f := range inputFields {
		fields[i] = FieldInfo{i, f, strings.Split(f, ".")}
	}

	// By using the FieldInfo type, once fields are sorted alphabetically, we maintain the
	// information about the original position of fields. This is used to check that array
	// indexes aren't out of order. Sorting the list of fields means for each field we only
	// have to compare it against two other fields at most to ensure validity of the whole set.
	sort.Slice(fields, func(i, j int) bool { return fields[i].field < fields[j].field })

	for index, fieldInfo := range fields {
		field := fieldInfo.field

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

		if index+1 < len(fields) {
			// For each field except the last, we check that the next field does not
			// contain the current field as a prefix. Since fields is sorted, this checks
			// case (3) and case (4).
			nextField := fields[index+1].field

			if strings.HasPrefix(nextField, field+".") {
				return fmt.Errorf("fields '%v' and '%v' are incompatible", field, nextField)
			}

			// This checks case (2).
			if field == nextField {
				return fmt.Errorf("fields cannot be identical: '%v' and '%v'", field, nextField)
			}
		}

		if useArrayIndexFields {
			var nextFieldInfo *FieldInfo
			var previousFieldInfo *FieldInfo
			if index+1 < len(fields) {
				nextFieldInfo = &fields[index+1]
			}
			if index != 0 {
				previousFieldInfo = &fields[index-1]
			}
			// This checks cases (5), (6), (7), and (8). This is achieved by comparing each field against the
			// next field and the previous field in the sorted list.
			err := validateFieldsUsingArrayIndexes(&fieldInfo, nextFieldInfo, previousFieldInfo, index, len(fields))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// validateFieldsUsingArrayIndexes is a helper function foe validateFields. It checks for
// cases (5), (6), (7), and (8). See validateFields for more information on the cases being checked.
func validateFieldsUsingArrayIndexes(fieldInfo *FieldInfo, nextFieldInfo *FieldInfo, previousFieldInfo *FieldInfo, fieldIndex int, numFields int) (err error) {
	fieldParts := fieldInfo.parts
	field := fieldInfo.field

	for i, part := range fieldParts {
		if i == 0 {
			// We don't need to do any checks at the root of the document.
			// This is because at the root of the document, numerical keys
			// aren't treated as array indexes.
			// i.e. the CSV `0\nhello` transforms to BSON `{"0": "hello"}``
			continue
		}

		// Only conduct checks if field part is a natural number (and therefore an array index)
		if num, ok := isNatNum(part); ok {
			if fieldIndex+1 < numFields {
				// Check against the next field if we're not already at the last field.
				// When we check against the next field, we're checking for cases (5), (7), and (8):
				// Case (5) is only partially covered, the rest of case (5) is covered by checkAgainstPreviousField.
				err = checkAgainstNextField(fieldInfo, nextFieldInfo, num, i)
				if err != nil {
					return err
				}
			}
			if fieldIndex != 0 {
				// Check against the previous field if we aren't currently looking at the first field.
				// When we check against the previous field, we're checking for cases (5), (6), and (8).
				// Case (5) is partially covered, the rest of case (5) is covered by checkAgainstNextField.
				// Case (8) was covered by checkAgainstNextField running on the previous field, so this is an extra check.
				err = checkAgainstPreviousField(fieldInfo, previousFieldInfo, num, i)
				if err != nil {
					return err
				}
			} else {
				if num != 0 {
					// Can't have non-zero index in very first value, this violates case (6).
					return fmt.Errorf("Array indexes must start from 0. No index found before index '%v' in field '%v'", part, field)
				}
			}

		}
	}
	return nil
}

// checkAgainstNextField performs some validity checks against the next field. It checks the following cases:
//
// - case (5): The current field part implies an array but the same part in the next field implies a document.
// - case (7): Array indexes are out of order. Although the index in the next field is sequential,
//             its position in the source is before the index in the current field.
// - case (8): The next index isn't sequential.
//
// See validateFields for more information on the cases being checked.
func checkAgainstNextField(fieldInfo *FieldInfo, nextFieldInfo *FieldInfo, num int, partPos int) error {
	field := fieldInfo.field
	fieldParts := fieldInfo.parts
	nextField := nextFieldInfo.field
	nextFieldParts := nextFieldInfo.parts

	if len(nextFieldParts) < partPos+1 {
		// If the next field has fewer field parts than the current field, they can't be inconsistent.
		// Note, we can infer this because the fields are sorted.
		return nil
	}
	if strings.Join(nextFieldParts[:partPos], ".") == strings.Join(fieldParts[:partPos], ".") {
		if nextNum, ok := isNatNum(nextFieldParts[partPos]); ok {
			if nextNum != num+1 && nextNum != num {
				// Case (8)
				return fmt.Errorf("field '%v' contains an array index that does not increase sequentially from field '%v'. "+
					"Array indexes in fields must start from 0 and increase sequentially", nextField, field)
			}
			if nextNum == num+1 && fieldInfo.position > nextFieldInfo.position {
				// Case (7)
				return fmt.Errorf("field '%v' comes before field '%v'. Array indexes must increase sequentially", nextField, field)
			}
		} else {
			// Case (5)
			return fmt.Errorf("fields '%v' and '%v' are incompatible", field, nextField)
		}
	}
	return nil
}

// checkAgainstPreviousField performs some validity checks against the last field. It checks the following cases:
//
// - case (5): The current field part implies an array but the same part in the last field implies a document.
//   This case can only happen when a document key gets sorted before natural numbers. So this is if a key
//   begins with special characters whose values are less than \x30 ('0') such as '\', '-', '!', '(', etc.
// - case (6): If the previous field doesn't share a prefix with the current field, this must be the first time
//   we are encountering an array. If the index isn't 0 we should throw an error.
// - case (8): We check the previous field's index is one less than the current field. This check was already done by
//   checkAgainstNextField on the previous iteration. The check is included here for an extra layer of defense.
//
// See validateFields for more information on the cases being checked.
func checkAgainstPreviousField(fieldInfo *FieldInfo, previousFieldInfo *FieldInfo, num int, partPos int) error {
	field := fieldInfo.field
	fieldParts := fieldInfo.parts
	previousField := previousFieldInfo.field
	previousFieldParts := previousFieldInfo.parts

	if len(previousFieldParts) < partPos+1 {
		// If the previous field has fewer field parts than the current field, they can't be inconsistent at the current partPos.
		// However, it also means this is the first time we are encountering this array (since fields are sorted).
		// This the index at the current partPos should be 0 or else it's invalid due to case (6).
		if num != 0 {
			return fmt.Errorf("Array indexes must start from 0. No index found before index '%v' in field '%v'", num, field)
		}
		return nil
	}
	if strings.Join(previousFieldParts[:partPos], ".") == strings.Join(fieldParts[:partPos], ".") {
		if lastNum, ok := isNatNum(previousFieldParts[partPos]); ok {
			if num != 0 && lastNum != num-1 && lastNum != num {
				// Looks back to check case (8)
				return fmt.Errorf("field '%v' contains an array index that does not increase sequentially from field '%v'. "+
					"Array indexes in fields must start from 0 and increase sequentially", field, previousField)
			}
		} else {
			// Case (5)
			return fmt.Errorf("fields '%v' and '%v' are incompatible", field, previousField)
		}
	} else if num != 0 {
		// Case (6)
		return fmt.Errorf("Array indexes must start from 0. No index found before index '%v' in field '%v'", num, field)
	}
	return nil
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
