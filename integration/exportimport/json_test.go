// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package exportimport

import (
	"encoding/base64"
	"math"
	"os"
	"strings"
	"time"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestRoundTripFieldsJSON verifies that mongoexport --fields limits which fields
// appear in JSON export output, and that _id is included (unlike CSV).
func (s *ExportImportSuite) TestRoundTripFieldsJSON() {
	const dbName = "mongoimport_roundtrip_fieldsjson_test"

	client := s.Client()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	s.Require().NoError(err)

	s.exportJSONAndImport(dbName, "a", db)
	dest := db.Collection("dest")
	n, err := dest.CountDocuments(s.Context(), bson.D{{"a", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(3, n, "3 documents should have a=1")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "b=1 should not have been exported")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 2}})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "b=2 should not have been exported")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"c", 3}})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "c=3 should not have been exported")

	s.exportJSONAndImport(dbName, "a,b,c", db)
	n, err = dest.CountDocuments(s.Context(), bson.D{{"a", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(3, n, "3 documents should have a=1")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 1}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "1 document should have b=1")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"b", 2}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "1 document should have b=2")
	n, err = dest.CountDocuments(s.Context(), bson.D{{"c", 3}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "1 document should have c=3")

	var fromSource, fromDest bson.M
	q := bson.D{{"a", 1}, {"b", 1}}
	err = db.Collection("source").FindOne(s.Context(), q).Decode(&fromSource)
	s.Require().NoError(err)
	err = dest.FindOne(s.Context(), q).Decode(&fromDest)
	s.Require().NoError(err)
	s.Assert().Equal(
		fromSource["_id"], fromDest["_id"],
		"_id should have been exported in JSON mode",
	)
}

// TestRoundTripJSONArray verifies that mongoexport --jsonArray produces a JSON
// array, that mongoimport rejects it without --jsonArray, and accepts it with
// --jsonArray.
func (s *ExportImportSuite) TestRoundTripJSONArray() {
	const dbName = "mongoimport_roundtrip_jsonarray_test"
	const collName = "data"

	client := s.Client()

	coll := client.Database(dbName).Collection(collName)
	docs := make([]any, 20)
	for i := range 20 {
		docs[i] = bson.D{{"_id", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err)

	exportToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
			JSONArray:  true,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	_, err = me.Export(tmpFile)
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	s.Require().NoError(coll.Drop(s.Context()))

	importWithoutFlagOpts, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	importWithoutFlagOpts.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   importWithoutFlagOpts,
		InputOptions:  &mongoimport.InputOptions{File: tmpFile.Name(), ParseGrace: "stop"},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Assert().Error(err, "import without --jsonArray should fail on jsonArray output")

	n, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err)
	s.Assert().EqualValues(0, n, "nothing should have been imported without --jsonArray")

	importToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err = mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			ParseGrace: "stop",
			JSONArray:  true,
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	imported, _, err := mi.ImportDocuments()
	s.Require().NoError(err)
	s.Assert().EqualValues(20, imported, "should import all 20 documents with --jsonArray")

	n, err = coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err)
	s.Assert().EqualValues(20, n, "all 20 documents should be present after import")
	for i := range 20 {
		c, err := coll.CountDocuments(s.Context(), bson.D{{"_id", i}})
		s.Require().NoError(err)
		s.Assert().EqualValues(1, c, "document with _id %d should exist", i)
	}
}

// legacyTypesJSON is legacy extended JSON, which is why it can spell a value as
// BinData(...) or NumberInt(5) or a bare hex literal.
const legacyTypesJSON = `{   "double_type" : 5.0,
    "double_exponent_type" : 5e+32,
    "double_negative_type" : -5.0,
    "NaN": NaN,
    "infinity" : Infinity,
    "negative_infinity" : -Infinity,
    "string_type" : "sample string",
    "object_type" : {"sample" : "object"},
    "binary_data" : BinData(3, "e8MEnzZoFyMmD7WSHdNrFJyEk8M="),
    "undefined_type" : undefined,
    "object_id_type" : ObjectId("54b03ef2a817f4f960f5b809"),
    "true_type" : true,
    "false_type" : false,
    "date_type" : Date(45),
    "iso_date_type" : ISODate("2015-02-25T16:42:11Z"),
    "null_type" : null,
    "int32_type" : 5,
    "int32_negative_type" : -5,
    "number_int_type" : NumberInt(5),
    "int32_hex" : 0x123,
    "int64_type" : 214748364765,
    "int64_negative_type" : -214748364765,
    "number_long_type" : NumberLong(5000),
    "minkey_type" : { "$minKey" : 1 },
    "maxkey_type" : { "$maxKey" : 1 },
    "regex_type" : { "$regex" : "\\.", "$options" : "" }
}
`

// TestImportLegacyExtendedJSONTypes checks that --legacy accepts every legacy
// extended-JSON spelling of a BSON value and stores each one as the value and
// the type that spelling denotes. Comparing the decoded values rather than
// querying by $type catches a hex literal parsed as 0 or a Date(45) parsed as
// the epoch, both of which still have the right type. Because the Go type of
// each expected value is the BSON type it decodes from, the comparison also
// subsumes the type checks — an int32 stored where an int64 was written no
// longer compares equal.
func (s *ExportImportSuite) TestImportLegacyExtendedJSONTypes() {
	const dbName = "mongoimport_legacy_types_test"

	coll := s.Client().Database(dbName).Collection("types")
	path := s.writeInputFile("types.json", legacyTypesJSON)

	ns := &options.Namespace{DB: dbName, Collection: "types"}
	s.Require().EqualValues(
		1,
		s.importJSONFile(ns, path, importFlags{legacy: true}),
		"the legacy extended JSON document is imported",
	)

	var imported bson.M
	s.Require().NoError(coll.FindOne(s.Context(), bson.D{}).Decode(&imported))

	binaryData, err := base64.StdEncoding.DecodeString("e8MEnzZoFyMmD7WSHdNrFJyEk8M=")
	s.Require().NoError(err)
	objectID, err := bson.ObjectIDFromHex("54b03ef2a817f4f960f5b809")
	s.Require().NoError(err)

	expected := []struct {
		field string
		value any
	}{
		{"double_type", 5.0},
		{"double_exponent_type", 5e+32},
		{"double_negative_type", -5.0},
		{"infinity", math.Inf(1)},
		{"negative_infinity", math.Inf(-1)},
		{"string_type", "sample string"},
		{"object_type", bson.D{{"sample", "object"}}},
		{"binary_data", bson.Binary{Subtype: 3, Data: binaryData}},
		{"undefined_type", bson.Undefined{}},
		{"object_id_type", objectID},
		{"true_type", true},
		{"false_type", false},
		{"date_type", bson.NewDateTimeFromTime(time.UnixMilli(45).UTC())},
		{
			"iso_date_type",
			bson.NewDateTimeFromTime(time.Date(2015, 2, 25, 16, 42, 11, 0, time.UTC)),
		},
		{"int32_type", int32(5)},
		{"int32_negative_type", int32(-5)},
		{"number_int_type", int32(5)},
		{"int32_hex", int32(0x123)},
		{"int64_type", int64(214748364765)},
		{"int64_negative_type", int64(-214748364765)},
		{"number_long_type", int64(5000)},
		{"minkey_type", bson.MinKey{}},
		{"maxkey_type", bson.MaxKey{}},
		{"regex_type", bson.Regex{Pattern: `\.`}},
	}
	for _, e := range expected {
		s.Assert().Equal(e.value, imported[e.field], "%s is stored as %#v", e.field, e.value)
	}

	// NaN never compares equal to itself, and a null is indistinguishable from an
	// absent field once decoded, so these two are checked on their own terms.
	nan, ok := imported["NaN"].(float64)
	if s.Assert().True(ok, "NaN is stored as a double") {
		s.Assert().True(math.IsNaN(nan), "NaN is stored as a NaN")
	}
	n, err := coll.CountDocuments(s.Context(), bson.D{{"null_type", bson.D{{"$type", "null"}}}})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, n, "null_type is stored as a null rather than left out")
}

// TestRoundTripJSONArrayOverMaxBSONSize checks that --jsonArray handles an
// export whose single array is far larger than the 16MB maximum BSON document
// size, in both directions: mongoexport has to stream the array out rather than
// build it, and mongoimport has to read documents out of it incrementally.
func (s *ExportImportSuite) TestRoundTripJSONArrayOverMaxBSONSize() {
	const dbName = "mongoimport_jsonarray_bigarray_test"
	const targetBytes = 20 * 1024 * 1024

	db := s.Client().Database(dbName)
	filler := strings.Repeat("a", 1024)
	sizedDoc, err := bson.Marshal(bson.D{{"_id", bson.NewObjectID()}, {"x", filler}})
	s.Require().NoError(err)
	numDocs := targetBytes / len(sizedDoc)

	docs := make([]any, numDocs)
	for i := range numDocs {
		docs[i] = bson.D{{"x", filler}}
	}
	_, err = db.Collection("source").InsertMany(s.Context(), docs)
	s.Require().NoError(err)

	exportFile := s.exportJSONFile(&options.Namespace{DB: dbName, Collection: "source"}, true)
	info, err := os.Stat(exportFile)
	s.Require().NoError(err)
	s.Require().Greater(
		info.Size(), int64(16*1024*1024),
		"the exported array is larger than the maximum BSON document size",
	)

	destNS := &options.Namespace{DB: dbName, Collection: "dest"}
	s.Assert().EqualValues(
		numDocs,
		s.importJSONFile(destNS, exportFile, importFlags{jsonArray: true}),
		"every document in the oversized array is imported",
	)

	// Comparing the two slices with a single Equal would dump both collections —
	// tens of megabytes — into the failure message, leaving the one document that
	// differs impossible to find, so they are walked in step instead.
	sourceDocs := s.docsSortedByID(db.Collection("source"))
	destDocs := s.docsSortedByID(db.Collection("dest"))
	s.Require().Len(destDocs, len(sourceDocs), "both collections hold the same number of documents")
	for i := range sourceDocs {
		if !s.Assert().Equal(sourceDocs[i], destDocs[i], "document %d survives the round trip", i) {
			break
		}
	}
}

func (s *ExportImportSuite) exportJSONAndImport(dbName, fields string, db *mongo.Database) {
	s.Require().NoError(db.Collection("dest").Drop(s.Context()))

	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	exportToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
			Fields:     fields,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	f, err := os.OpenFile(tmpFile.Name(), os.O_WRONLY, 0o644)
	s.Require().NoError(err)
	_, err = me.Export(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	importToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)
}
