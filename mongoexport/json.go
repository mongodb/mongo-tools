// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"bytes"
	"encoding/hex"
	"io"
	"reflect"

	"github.com/mongodb/mongo-tools/common/json"
	errors2 "github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// JSONExportOutput is an implementation of ExportOutput that writes documents
// to the output in JSON format.
type JSONExportOutput struct {
	// ArrayOutput when set to true indicates that the output should be written
	// as a JSON array, where each document is an element in the array.
	ArrayOutput bool
	// Pretty when set to true indicates that the output will be written in pretty mode.
	PrettyOutput bool
	Out          io.Writer
	NumExported  int64
	JSONFormat   JSONFormat
}

// NewJSONExportOutput creates a new JSONExportOutput in array mode if specified,
// configured to write data to the given io.Writer.
func NewJSONExportOutput(
	arrayOutput bool,
	prettyOutput bool,
	out io.Writer,
	jsonFormat JSONFormat,
) *JSONExportOutput {
	return &JSONExportOutput{
		arrayOutput,
		prettyOutput,
		out,
		0,
		jsonFormat,
	}
}

// WriteHeader writes the opening square bracket if in array mode, otherwise it
// behaves as a no-op.
func (jsonExporter *JSONExportOutput) WriteHeader() error {
	if jsonExporter.ArrayOutput {
		// TODO check # bytes written?
		_, err := jsonExporter.Out.Write([]byte{json.ArrayStart})
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteFooter writes the closing square bracket if in array mode, otherwise it
// behaves as a no-op.
func (jsonExporter *JSONExportOutput) WriteFooter() error {
	if jsonExporter.ArrayOutput {
		_, err := jsonExporter.Out.Write([]byte{json.ArrayEnd, '\n'})
		// TODO check # bytes written?
		if err != nil {
			return err
		}
	}
	if jsonExporter.PrettyOutput {
		if _, err := jsonExporter.Out.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

// Flush is a no-op for JSON export formats.
func (jsonExporter *JSONExportOutput) Flush() error {
	return nil
}

// ExportDocument converts the given document to extended JSON, and writes it
// to the output.
func (jsonExporter *JSONExportOutput) ExportDocument(document bson.D) error {
	if jsonExporter.ArrayOutput || jsonExporter.PrettyOutput {
		if jsonExporter.NumExported >= 1 {
			if jsonExporter.ArrayOutput {
				if _, err := jsonExporter.Out.Write([]byte(",")); err != nil {
					return err
				}
			}
			if jsonExporter.PrettyOutput {
				if _, err := jsonExporter.Out.Write([]byte("\n")); err != nil {
					return err
				}
			}
		}

		jsonOut, err := marshalExtJSONUUIDString(
			document,
			jsonExporter.JSONFormat == Canonical,
			false,
		)
		if err != nil {
			return err
		}

		if jsonExporter.PrettyOutput {
			var jsonFormatted bytes.Buffer
			if err = json.Indent(&jsonFormatted, jsonOut, "", "\t"); err != nil {
				return err
			}
			jsonOut = jsonFormatted.Bytes()
		}

		if _, err = jsonExporter.Out.Write(jsonOut); err != nil {
			return err
		}
	} else {
		extendedDoc, err := marshalExtJSONUUIDString(document, jsonExporter.JSONFormat == Canonical, false)
		if err != nil {
			return err
		}

		extendedDoc = append(extendedDoc, '\n')
		if _, err = jsonExporter.Out.Write(extendedDoc); err != nil {
			return err
		}
	}
	jsonExporter.NumExported++
	return nil
}

var tBinary = reflect.TypeOf(primitive.Binary{})

func uuidString(uuid []byte) string {
	var str [36]byte
	hex.Encode(str[:], uuid[:4])
	str[8] = '-'
	hex.Encode(str[9:13], uuid[4:6])
	str[13] = '-'
	hex.Encode(str[14:18], uuid[6:8])
	str[18] = '-'
	hex.Encode(str[19:23], uuid[8:10])
	str[23] = '-'
	hex.Encode(str[24:], uuid[10:])
	return string(str[:])
}

// binaryEncodeUUIDString is the ValueEncoderFunc for Binary.
func binaryEncodeUUIDString(_ bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != tBinary {
		return bsoncodec.ValueEncoderError{Name: "binaryEncodeUUIDString", Types: []reflect.Type{tBinary}, Received: val}
	}
	b := val.Interface().(primitive.Binary)

	if b.Subtype == bson.TypeBinaryUUID {
		dw, err := vw.WriteDocument()
		if err != nil {
			return err
		}
		vw, err := dw.WriteDocumentElement("$uuid")
		if err != nil {
			return err
		}

		err = vw.WriteString(uuidString(b.Data))
		if err != nil {
			return err
		}
		return dw.WriteDocumentEnd()
	}

	return vw.WriteBinaryWithSubtype(b.Data, b.Subtype)
}

var uuidStringReg *bsoncodec.Registry

func init() {
	uuidStringReg = bson.NewRegistry()
	uuidStringReg.RegisterTypeEncoder(tBinary, bsoncodec.ValueEncoderFunc(binaryEncodeUUIDString))
}

func marshalExtJSONUUIDString(val interface{}, canonical bool, escapeHTML bool) ([]byte, error) {
	buf := new(bytes.Buffer)
	vw, err := bsonrw.NewExtJSONValueWriter(buf, canonical, escapeHTML)
	if err != nil {
		return nil, err
	}
	enc, err := bson.NewEncoder(vw)
	if err != nil {
		return nil, err
	}
	enc.SetRegistry(uuidStringReg)
	err = enc.Encode(val)
	if err != nil {
		return nil, err
	}
	jsonBytes := buf.Bytes()

	reversedVal := reflect.New(reflect.TypeOf(val)).Elem().Interface()
	if unmarshalErr := bson.UnmarshalExtJSON(jsonBytes, canonical, &reversedVal); unmarshalErr != nil {
		return nil, errors2.Wrap(unmarshalErr, "marshal is not reversible")
	}
	return jsonBytes, nil
}
