// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package exportimport

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestRoundTripFieldFile verifies that mongoexport --fieldFile limits exported
// fields, and that mongoimport correctly restores the filtered data.
func (s *ExportImportSuite) TestRoundTripFieldFile() {
	const dbName = "mongoimport_roundtrip_fieldfile_test"

	client := s.Client()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	s.Require().NoError(err)

	fieldFile, err := os.CreateTemp(s.T().TempDir(), "fields-*.txt")
	s.Require().NoError(err)
	_, err = fieldFile.WriteString("a\nb\n")
	s.Require().NoError(err)
	s.Require().NoError(fieldFile.Close())

	exportTarget, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err)
	s.Require().NoError(exportTarget.Close())

	exportToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			FieldFile:  fieldFile.Name(),
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()

	f, err := os.OpenFile(exportTarget.Name(), os.O_WRONLY, 0o644)
	s.Require().NoError(err)
	_, err = me.Export(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	fields := "a,b,c"
	importToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       exportTarget.Name(),
			Type:       "csv",
			Fields:     &fields,
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)

	dest := db.Collection("dest")
	n, err := dest.CountDocuments(s.Context(), bson.D{{"a", 1}})
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
	s.Assert().EqualValues(0, n, "c=3 should not have been exported (not in fieldFile)")
}

// TestRoundTripFieldsCSV verifies that mongoexport --csv --fields limits which
// fields are exported, and that mongoimport correctly restores the filtered data.
func (s *ExportImportSuite) TestRoundTripFieldsCSV() {
	const dbName = "mongoimport_roundtrip_fieldscsv_test"

	client := s.Client()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 1}, {"b", 1}},
		bson.D{{"a", 1}, {"b", 2}, {"c", 3}},
	})
	s.Require().NoError(err)

	s.exportCSVAndImport(dbName, "a", db)
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

	s.exportCSVAndImport(dbName, "a,b,c", db)
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
	s.Assert().NotEqual(fromSource["_id"], fromDest["_id"], "_id should not have been exported")
}

// TestRoundTripNestedFieldsCSV verifies that mongoexport correctly exports
// nested dotted field paths to CSV and that mongoimport restores them.
func (s *ExportImportSuite) TestRoundTripNestedFieldsCSV() {
	const dbName = "mongoimport_roundtrip_nestedcsv_test"

	client := s.Client()

	db := client.Database(dbName)
	_, err := db.Collection("source").InsertMany(s.Context(), []any{
		bson.D{{"a", 1}},
		bson.D{{"a", 2}, {"b", bson.D{{"c", 2}}}},
		bson.D{{"a", 3}, {"b", bson.D{{"c", 3}, {"d", bson.D{{"e", 3}}}}}},
		bson.D{{"a", 4}, {"x", nil}},
	})
	s.Require().NoError(err)

	exportToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			Fields:     "a,b.d.e,x.y",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	tmpFile, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err)
	_, err = me.Export(tmpFile)
	s.Require().NoError(err)
	s.Require().NoError(tmpFile.Close())

	importToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       tmpFile.Name(),
			Type:       "csv",
			HeaderLine: true,
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)

	dest := db.Collection("dest")
	for _, tc := range []struct {
		filter bson.D
		count  int64
		msg    string
	}{
		{bson.D{{"b.c", 2}}, 0, "b.c should not have been exported"},
		{bson.D{{"b.c", 3}}, 0, "b.c should not have been exported"},
		{bson.D{{"b.d.e", 3}}, 1, "b.d.e=3 should be present"},
		{bson.D{{"b.d.e", ""}}, 3, "b.d.e should be empty string for 3 docs"},
		{bson.D{{"a", 1}}, 1, "a=1 should be present"},
		{bson.D{{"a", 2}}, 1, "a=2 should be present"},
		{bson.D{{"a", 3}}, 1, "a=3 should be present"},
		{bson.D{{"x.y", ""}}, 4, "x.y should be empty string for all 4 docs"},
	} {
		n, err := dest.CountDocuments(s.Context(), tc.filter)
		s.Require().NoError(err)
		s.Assert().EqualValues(tc.count, n, tc.msg)
	}
}

// csvPunctuationFields are the fields of csvPunctuationDoc, including the one
// whose name holds a space.
const csvPunctuationFields = "a,b,c,d d,e,f"

// csvPunctuationDoc holds values that CSV has to quote or escape: a value with
// both commas and embedded double quotes, values that are nothing but
// punctuation, and a field name containing a space.
var csvPunctuationDoc = bson.M{
	"a":   int32(1),
	"b":   `foo,bar"baz,qux`,
	"c":   int32(5),
	"d d": int32(-6),
	"e":   "-",
	"f":   ".",
}

// TestCSVRoundTripPunctuation checks that values needing CSV quoting survive an
// export and import unchanged, and that the exported header row is only treated
// as a header when --headerline says so.
func (s *ExportImportSuite) TestCSVRoundTripPunctuation() {
	const dbName = "exportimport_csv_punctuation"

	testDB := s.Client().Database(dbName)
	_, err := testDB.Collection("source").InsertOne(s.Context(), csvPunctuationDoc)
	s.Require().NoError(err, "can insert the document to export")

	csvPath := s.exportCSV(
		&options.Namespace{DB: dbName, Collection: "source"},
		csvPunctuationFields,
		"",
	)

	s.Run("fields option treats the header row as data", func() {
		ns := &options.Namespace{DB: dbName, Collection: "with_fields"}
		s.importDelimited(ns, csvPath, "csv", csvPunctuationFields)

		docs := s.docsSortedByA(testDB.Collection("with_fields"))
		s.Require().Len(docs, 2, "the data row and the header row are both imported")
		s.Assert().Equal(csvPunctuationDoc, docs[0], "the data row round trips unchanged")
		s.Assert().Equal(
			bson.M{"a": "a", "b": "b", "c": "c", "d d": "d d", "e": "e", "f": "f"},
			docs[1],
			"without --headerline the header row is imported as an ordinary document",
		)
	})

	s.Run("headerline option consumes the header row", func() {
		ns := &options.Namespace{DB: dbName, Collection: "with_headerline"}
		s.importDelimited(ns, csvPath, "csv", "")

		docs := s.docsSortedByA(testDB.Collection("with_headerline"))
		s.Require().Len(docs, 1, "only the data row is imported")
		s.Assert().Equal(csvPunctuationDoc, docs[0], "the data row round trips unchanged")
	})
}

// csvTextEdgeCases is a CSV whose irregular whitespace is the point: cells are
// variously padded outside their quotes, and the parser has to strip that padding
// but keep whatever is inside the quotes.
const csvTextEdgeCases = "a,b,c\n" +
	"1,\"this is some text.\n" +
	"This text spans multiple lines, and just for fun\n" +
	"contains a comma\",    \"This has leading and trailing whitespace!\"  \n" +
	"2, \"When someone says something you \"\"put it in quotes\"\"\", I like embedded quotes/slashes\\backslashes  \n" +
	"  3  , \"  This line contains the empty string and has leading and trailing whitespace inside the quotes!  \", \"\"\n" +
	" \"4\" ,,  How are empty entries handled?  \n" +
	"\"5\",\"\"\"\"\"\", \"\"\"This string is in quotes and contains empty quotes (\"\"\"\")\"\"\"\n"

// csvTextEdgeCaseDocs are the documents csvTextEdgeCases has to import as,
// ordered by their a field.
var csvTextEdgeCaseDocs = []bson.M{
	{
		"a": int32(1),
		"b": "this is some text.\nThis text spans multiple lines, and just for fun\ncontains a comma",
		"c": "This has leading and trailing whitespace!",
	},
	{
		"a": int32(2),
		"b": `When someone says something you "put it in quotes"`,
		"c": `I like embedded quotes/slashes\backslashes`,
	},
	{
		"a": int32(3),
		"b": "  This line contains the empty string and has leading and trailing whitespace inside the quotes!  ",
		"c": "",
	},
	{"a": int32(4), "b": "", "c": "How are empty entries handled?"},
	{
		"a": int32(5),
		"b": `""`,
		"c": `"This string is in quotes and contains empty quotes ("")"`,
	},
}

// TestCSVImportTextEdgeCases checks how the CSV parser handles newlines and
// commas inside quoted cells, doubled quotes, backslashes, empty cells, and
// padding around quoted cells.
func (s *ExportImportSuite) TestCSVImportTextEdgeCases() {
	const dbName = "exportimport_csv_text_edges"

	testDB := s.Client().Database(dbName)
	csvPath := s.writeInputFile("csvimport1.csv", csvTextEdgeCases)

	s.Run("fields option treats the header row as data", func() {
		ns := &options.Namespace{DB: dbName, Collection: "with_fields"}
		s.importDelimited(ns, csvPath, "csv", "a,b,c")

		docs := s.docsSortedByA(testDB.Collection("with_fields"))
		s.Require().
			Len(docs, len(csvTextEdgeCaseDocs)+1, "every row including the header is imported")
		s.Assert().Equal(
			append(slices.Clone(csvTextEdgeCaseDocs), bson.M{"a": "a", "b": "b", "c": "c"}),
			docs,
			"each row imports as its own document, with the header row among them",
		)
	})

	s.Run("headerline option consumes the header row", func() {
		ns := &options.Namespace{DB: dbName, Collection: "with_headerline"}
		s.importDelimited(ns, csvPath, "csv", "")

		docs := s.docsSortedByA(testDB.Collection("with_headerline"))
		s.Assert().Equal(csvTextEdgeCaseDocs, docs, "every row but the header imports unchanged")
	})
}

// tsvTextEdgeCases has an empty first cell, which is what distinguishes an empty
// TSV cell from a missing one.
const tsvTextEdgeCases = "a\tb\tc\td\te\n" +
	"\t1\tfoobar\t5\t-6\n"

var tsvTextEdgeCasesDoc = bson.M{
	"a": "",
	"b": int32(1),
	"c": "foobar",
	"d": int32(5),
	"e": int32(-6),
}

// TestTSVImport checks that a tab-delimited file imports with both an explicit
// --fields list and --headerline.
func (s *ExportImportSuite) TestTSVImport() {
	const dbName = "exportimport_tsv"

	testDB := s.Client().Database(dbName)
	tsvPath := s.writeInputFile("a.tsv", tsvTextEdgeCases)

	s.Run("fields option treats the header row as data", func() {
		ns := &options.Namespace{DB: dbName, Collection: "with_fields"}
		s.importDelimited(ns, tsvPath, "tsv", "a,b,c,d,e")

		docs := s.docsSortedByA(testDB.Collection("with_fields"))
		s.Require().Len(docs, 2, "the data row and the header row are both imported")
		s.Assert().Equal(tsvTextEdgeCasesDoc, docs[0], "the data row imports unchanged")
		s.Assert().Equal(
			bson.M{"a": "a", "b": "b", "c": "c", "d": "d", "e": "e"},
			docs[1],
			"without --headerline the header row is imported as an ordinary document",
		)
	})

	s.Run("headerline option consumes the header row", func() {
		ns := &options.Namespace{DB: dbName, Collection: "with_headerline"}
		s.importDelimited(ns, tsvPath, "tsv", "")

		docs := s.docsSortedByA(testDB.Collection("with_headerline"))
		s.Require().Len(docs, 1, "only the data row is imported")
		s.Assert().Equal(tsvTextEdgeCasesDoc, docs[0], "the data row imports unchanged")
	})
}

// TestCSVExportFormatsBSONTypes checks how mongoexport renders BSON types that
// have no CSV equivalent, and that mongoimport can read those renderings back.
// Exporting to CSV is lossy by design, so the export half pins the exact text of
// each cell instead of expecting the values to survive.
func (s *ExportImportSuite) TestCSVExportFormatsBSONTypes() {
	const dbName = "exportimport_csv_export_types"

	objID := bson.NewObjectID()
	binary, err := base64.StdEncoding.DecodeString("1234")
	s.Require().NoError(err, "can decode the binary payload")
	when, err := time.Parse(time.RFC3339Nano, "2009-08-27T12:34:56.789Z")
	s.Require().NoError(err, "can parse the date")

	testDB := s.Client().Database(dbName)
	_, err = testDB.Collection("source").InsertMany(s.Context(), []any{
		bson.D{
			{"_id", 1},
			{"a", int32(1)},
			{"b", objID},
			{"c", bson.A{1, 2, 3}},
			{"d", bson.D{{"a", "hello"}, {"b", "world"}}},
			{"e", "-"},
		},
		bson.D{
			{"_id", 2},
			{"a", -2.0},
			{"c", bson.MinKey{}},
			{"d", `Then he said, "Hello World!"`},
			{"e", int64(3)},
		},
		bson.D{
			{"_id", 3},
			{"a", bson.Binary{Subtype: 0, Data: binary}},
			{"b", when},
			{"c", bson.Timestamp{T: 1234, I: 9876}},
			{"d", bson.Regex{Pattern: `foo*\"bar\"`, Options: "i"}},
			{"e", bson.JavaScript(`function foo() { print("Hello World!"); }`)},
		},
	})
	s.Require().NoError(err, "can insert the documents to export")

	// The expected text below is row by row, so the export has to be ordered rather
	// than left in the server's natural order.
	csvPath := s.exportCSV(
		&options.Namespace{DB: dbName, Collection: "source"},
		"a,b,c,d,e",
		`{"_id": 1}`,
	)
	exported, err := os.ReadFile(csvPath)
	s.Require().NoError(err, "can read the exported CSV")

	// Arrays and subdocuments become JSON in a single cell. An ObjectId keeps its
	// constructor syntax, MinKey becomes $MinKey, binary data becomes hex, a date
	// becomes ISO 8601, and a timestamp becomes extended JSON. A regex keeps its
	// delimiters and flags, and JavaScript keeps its source text. Doubled quotes
	// are CSV's own escaping of a quote inside a quoted cell.
	expected := strings.Join([]string{
		"a,b,c,d,e",
		`1,ObjectId(` + objID.Hex() + `),"[1,2,3]","{""a"":""hello"",""b"":""world""}",-`,
		`-2,,$MinKey,"Then he said, ""Hello World!""",3`,
		`D76DF8,2009-08-27T12:34:56.789Z,` +
			`"{ ""$timestamp"": { ""t"": 1234, ""i"": 9876 } }",` +
			`"/foo*\""bar\""/i","function foo() { print(""Hello World!""); }"`,
	}, "\n") + "\n"

	s.Assert().Equal(expected, string(exported), "each BSON type is rendered as expected")

	// Importing the export back checks the other direction: mongoexport writes cells
	// that mongoimport has to be able to parse, awkward ones included. None of the
	// original types survive, because the CSV holds only
	// their rendered text, so every cell that is not a bare number comes back as a
	// string.
	s.importDelimited(&options.Namespace{DB: dbName, Collection: "dest"}, csvPath, "csv", "")

	s.Assert().Equal(
		[]bson.M{
			{
				"a": int32(-2),
				"b": "",
				"c": "$MinKey",
				"d": `Then he said, "Hello World!"`,
				"e": int32(3),
			},
			{
				"a": int32(1),
				"b": "ObjectId(" + objID.Hex() + ")",
				"c": "[1,2,3]",
				"d": `{"a":"hello","b":"world"}`,
				"e": "-",
			},
			{
				"a": "D76DF8",
				"b": "2009-08-27T12:34:56.789Z",
				"c": `{ "$timestamp": { "t": 1234, "i": 9876 } }`,
				"d": `/foo*\"bar\"/i`,
				"e": `function foo() { print("Hello World!"); }`,
			},
		},
		s.docsSortedByA(testDB.Collection("dest")),
		"mongoimport parses every cell mongoexport wrote",
	)
}

func (s *ExportImportSuite) exportCSVAndImport(dbName, exportFields string, db *mongo.Database) {
	s.Require().NoError(db.Collection("dest").Drop(s.Context()))

	exportTarget, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err)
	s.Require().NoError(exportTarget.Close())

	exportToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "source"}
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			Fields:     exportFields,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	f, err := os.OpenFile(exportTarget.Name(), os.O_WRONLY, 0o644)
	s.Require().NoError(err)
	_, err = me.Export(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	importFields := "a,b,c"
	importToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: "dest"}
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: importToolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       exportTarget.Name(),
			Type:       "csv",
			Fields:     &importFields,
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err)
}

// exportCSV exports ns to a temp CSV file holding the named fields and returns
// the file's path. sort is a mongoexport --sort argument, needed when a test
// depends on the order of the exported rows; passing an empty sort leaves the
// server's natural order alone.
func (s *ExportImportSuite) exportCSV(ns *options.Namespace, fields, sort string) string {
	exportFile, err := os.CreateTemp(s.T().TempDir(), "export-*.csv")
	s.Require().NoError(err, "can create the file to export into")

	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	toolOptions.Namespace = ns

	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: toolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "csv",
			JSONFormat: "canonical",
			Fields:     fields,
		},
		InputOptions: &mongoexport.InputOptions{Sort: sort},
	})
	s.Require().NoError(err)
	defer me.Close()

	_, err = me.Export(exportFile)
	s.Require().NoError(err, "can export %#q as CSV", ns.Collection)
	s.Require().NoError(exportFile.Close())

	return exportFile.Name()
}

// importDelimited imports path into ns as the given type, either csv or tsv.
// fields names the columns; passing an empty fields uses the file's first line
// as the header instead.
func (s *ExportImportSuite) importDelimited(
	ns *options.Namespace,
	path, fileType, fields string,
) {
	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	toolOptions.Namespace = ns

	inputOptions := &mongoimport.InputOptions{
		File:       path,
		Type:       fileType,
		ParseGrace: "stop",
	}
	if fields == "" {
		inputOptions.HeaderLine = true
	} else {
		inputOptions.Fields = &fields
	}

	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   toolOptions,
		InputOptions:  inputOptions,
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	defer mi.Close()

	_, _, err = mi.ImportDocuments()
	s.Require().NoError(err, "can import %#q as %s", filepath.Base(path), fileType)
}

// docsSortedByA returns the documents in coll ordered by their a field, with
// their _ids removed so that they can be compared against expected documents.
// Every input here keys its rows on a and no two rows share a value, so the
// order is total. Where a row's a is a number, BSON's ordering of numbers ahead
// of strings puts an imported header row last; the TSV input instead relies on
// its empty first cell sorting before the header's "a".
func (s *ExportImportSuite) docsSortedByA(coll *mongo.Collection) []bson.M {
	cursor, err := coll.Find(
		s.Context(),
		bson.D{},
		mopt.Find().SetSort(bson.D{{"a", 1}}),
	)
	s.Require().NoError(err, "can read %#q", coll.Name())

	var docs []bson.M
	s.Require().NoError(cursor.All(s.Context(), &docs), "can decode %#q", coll.Name())

	for _, doc := range docs {
		delete(doc, "_id")
	}

	return docs
}

// writeInputFile writes content to a temp file named name and returns its path.
func (s *ExportImportSuite) writeInputFile(name, content string) string {
	path := filepath.Join(s.T().TempDir(), name)
	s.Require().NoError(os.WriteFile(path, []byte(content), 0o600), "can write %#q", name)

	return path
}
