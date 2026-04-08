// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDb         = "db"
	testCollection = "c"
	mioSoeFile     = "testdata/10k1dup10k.json"
)

// checkOnlyHasDocuments returns an error if the documents in the test
// collection don't exactly match those that are passed in.
func checkOnlyHasDocuments(
	t *testing.T,
	sessionProvider *db.SessionProvider,
	expectedDocuments []bson.M,
) error {
	session, err := sessionProvider.GetSession()
	if err != nil {
		return err
	}

	collection := session.Database(testDb).Collection(testCollection)
	cursor, err := collection.Find(
		t.Context(),
		bson.D{},
		mopt.Find().SetSort(bson.D{{"_id", 1}}),
	)
	if err != nil {
		return err
	}

	var docs []bson.M
	for cursor.Next(t.Context()) {
		decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(cursor.Current)))
		decoder.DefaultDocumentM()
		var doc bson.M
		if err := decoder.Decode(&doc); err != nil {
			return err
		}

		docs = append(docs, doc)
	}
	if len(docs) != len(expectedDocuments) {
		return fmt.Errorf("document count mismatch: expected %#v, got %#v",
			len(expectedDocuments), len(docs))
	}

	for index := range docs {
		if !reflect.DeepEqual(docs[index], expectedDocuments[index]) {
			return fmt.Errorf("document mismatch: expected %#v, got %#v",
				expectedDocuments[index], docs[index])
		}
	}

	return nil
}

func countDocuments(t *testing.T, sessionProvider *db.SessionProvider) (int, error) {
	session, err := (*sessionProvider).GetSession()
	if err != nil {
		return 0, err
	}

	collection := session.Database(testDb).Collection(testCollection)
	n, err := collection.CountDocuments(t.Context(), bson.D{})
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

// getBasicToolOptions returns a test helper to instantiate the session provider
// for calls to StreamDocument.
func getBasicToolOptions() *options.ToolOptions {
	general := &options.General{}
	ssl := testutil.GetSSLOptions()
	auth := testutil.GetAuthOptions()
	namespace := &options.Namespace{
		DB:         testDb,
		Collection: testCollection,
	}
	connection := &options.Connection{
		Host: "localhost",
		Port: db.DefaultTestPort,
	}

	return &options.ToolOptions{
		General:      general,
		SSL:          &ssl,
		Namespace:    namespace,
		Connection:   connection,
		Auth:         &auth,
		URI:          &options.URI{},
		WriteConcern: wcwrapper.Majority(),
	}
}

func newOptions() Options {
	return Options{
		ToolOptions: getBasicToolOptions(),
		InputOptions: &InputOptions{
			ParseGrace: "stop",
		},
		IngestOptions: &IngestOptions{},
	}
}

func NewMongoImport() (*MongoImport, error) {
	return New(newOptions())
}

// NewMockMongoImport gets an instance of MongoImport with no underlying SessionProvider.
// Use this for tests that don't communicate with the server (e.g. options parsing tests).
func NewMockMongoImport() *MongoImport {
	toolOptions := getBasicToolOptions()
	inputOptions := &InputOptions{
		ParseGrace: "stop",
	}
	ingestOptions := &IngestOptions{}

	return &MongoImport{
		ToolOptions:     toolOptions,
		InputOptions:    inputOptions,
		IngestOptions:   ingestOptions,
		SessionProvider: nil,
	}
}

func getImportWithArgs(additionalArgs ...string) (*MongoImport, error) {
	opts, err := ParseOptions(append(testutil.GetBareArgs(), additionalArgs...), "", "")
	if err != nil {
		return nil, fmt.Errorf("error parsing args: %v", err)
	}

	// Some OSes take longer than others. The test will time itself
	// out anyway, so we disable timeouts here.
	if opts.Timeout > 0 {
		fmt.Printf("getImportWithArgs zeroing timeout (was %v)\n", opts.Timeout)
		opts.Timeout = 0
	}

	imp, err := New(opts)
	if err != nil {
		return nil, fmt.Errorf("error making new instance of mongorestore: %v", err)
	}

	return imp, nil
}

func TestSplitInlineHeader(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("handle normal, untyped headers", t, func() {
		fields := []string{"foo.bar", "baz", "boo"}
		header := strings.Join(fields, ",")
		Convey("with '"+header+"'", func() {
			So(splitInlineHeader(header), ShouldResemble, fields)
		})
	})
	Convey("handle typed headers", t, func() {
		fields := []string{"foo.bar.string()", "baz.date(January 2 2006)", "boo.binary(hex)"}
		header := strings.Join(fields, ",")
		Convey("with '"+header+"'", func() {
			So(splitInlineHeader(header), ShouldResemble, fields)
		})
	})
	Convey("handle typed headers that include commas", t, func() {
		fields := []string{"foo.bar.date(,,,,)", "baz.date(January 2, 2006)", "boo.binary(hex)"}
		header := strings.Join(fields, ",")
		Convey("with '"+header+"'", func() {
			So(splitInlineHeader(header), ShouldResemble, fields)
		})
	})
}

func TestMongoImportValidateSettings(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("Given a mongoimport instance for validation, ", t, func() {
		Convey("an error should be thrown if no collection is given", func() {
			imp := NewMockMongoImport()
			imp.ToolOptions.DB = ""
			imp.ToolOptions.Collection = ""
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("an error should be thrown if an invalid type is given", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Type = "invalid"
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("an error should be thrown if neither --headerline is supplied "+
			"nor --fields/--fieldFile", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Type = CSV
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("no error should be thrown if --headerline is not supplied "+
			"but --fields is supplied", func() {
			imp := NewMockMongoImport()
			fields := "a,b,c"
			imp.InputOptions.Fields = &fields
			imp.InputOptions.Type = CSV
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("no error should be thrown if no input type is supplied", func() {
			imp := NewMockMongoImport()
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("no error should be thrown if there's just one positional argument", func() {
			imp := NewMockMongoImport()
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("no error should be thrown if --file is used with one positional argument", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.File = "abc"
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("no error should be thrown if there's more than one positional argument", func() {
			imp := NewMockMongoImport()
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("an error should be thrown if --headerline is used with JSON input", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("an error should be thrown if --fields is used with JSON input", func() {
			imp := NewMockMongoImport()
			fields := ""
			imp.InputOptions.Fields = &fields
			So(imp.validateSettings(), ShouldNotBeNil)
			fields = "a,b,c"
			imp.InputOptions.Fields = &fields
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("an error should be thrown if --fieldFile is used with JSON input", func() {
			imp := NewMockMongoImport()
			fieldFile := ""
			imp.InputOptions.FieldFile = &fieldFile
			So(imp.validateSettings(), ShouldNotBeNil)
			fieldFile = "test.csv"
			imp.InputOptions.FieldFile = &fieldFile
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("an error should be thrown if --ignoreBlanks is used with JSON input", func() {
			imp := NewMockMongoImport()
			imp.IngestOptions.IgnoreBlanks = true
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("no error should be thrown if --headerline is not supplied "+
			"but --fieldFile is supplied", func() {
			imp := NewMockMongoImport()
			fieldFile := "test.csv"
			imp.InputOptions.FieldFile = &fieldFile
			imp.InputOptions.Type = CSV
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("an error should be thrown if --mode is incorrect", func() {
			imp := NewMockMongoImport()
			imp.IngestOptions.Mode = "wrong"
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("an error should be thrown if a field in the --upsertFields "+
			"argument starts with a dollar sign", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.IngestOptions.Mode = modeUpsert
			imp.IngestOptions.UpsertFields = "a,$b,c"
			So(imp.validateSettings(), ShouldNotBeNil)
			imp.IngestOptions.UpsertFields = "a,.b,c"
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("no error should be thrown if --upsertFields is supplied without "+
			"--mode=xxx", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.IngestOptions.UpsertFields = "a,b,c"
			So(imp.validateSettings(), ShouldBeNil)
			So(imp.IngestOptions.Mode, ShouldEqual, modeUpsert)
		})

		Convey("an error should be thrown if --upsertFields is used with "+
			"--mode=insert", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.IngestOptions.Mode = modeInsert
			imp.IngestOptions.UpsertFields = "a"
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("if --mode=upsert is used without --upsertFields, _id should be set as "+
			"the upsert field", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.IngestOptions.Mode = modeUpsert
			imp.IngestOptions.UpsertFields = ""
			So(imp.validateSettings(), ShouldBeNil)
			So(imp.upsertFields, ShouldResemble, []string{"_id"})
		})

		Convey("if --mode=delete is used without --upsertFields, _id should be set as "+
			"the upsert field", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.IngestOptions.Mode = modeDelete
			imp.IngestOptions.UpsertFields = ""
			So(imp.validateSettings(), ShouldBeNil)
			So(imp.upsertFields, ShouldResemble, []string{"_id"})
		})

		Convey("no error should be thrown if all fields in the --upsertFields "+
			"argument are valid", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.IngestOptions.Mode = modeUpsert
			imp.IngestOptions.UpsertFields = "a,b,c"
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("no error should be thrown if --fields is supplied with CSV import", func() {
			imp := NewMockMongoImport()
			fields := "a,b,c"
			imp.InputOptions.Fields = &fields
			imp.InputOptions.Type = CSV
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey(
			"an error should be thrown if an empty --fields is supplied with CSV import",
			func() {
				imp := NewMockMongoImport()
				fields := ""
				imp.InputOptions.Fields = &fields
				imp.InputOptions.Type = CSV
				So(imp.validateSettings(), ShouldBeNil)
			},
		)

		Convey("no error should be thrown if --fieldFile is supplied with CSV import", func() {
			imp := NewMockMongoImport()
			fieldFile := "test.csv"
			imp.InputOptions.FieldFile = &fieldFile
			imp.InputOptions.Type = CSV
			So(imp.validateSettings(), ShouldBeNil)
		})

		Convey("an error should be thrown if no collection and no file is supplied", func() {
			imp := NewMockMongoImport()
			fieldFile := "test.csv"
			imp.InputOptions.FieldFile = &fieldFile
			imp.InputOptions.Type = CSV
			imp.ToolOptions.Collection = ""
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey("no error should be thrown if --file is used (without -c) supplied "+
			"- the file name should be used as the collection name", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.File = "input"
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.ToolOptions.Collection = ""
			So(imp.validateSettings(), ShouldBeNil)
			So(imp.ToolOptions.Collection, ShouldEqual,
				imp.InputOptions.File)
		})

		Convey("with no collection name and a file name the base name of the "+
			"file (without the extension) should be used as the collection name", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.File = "/path/to/input/file/dot/input.txt"
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.ToolOptions.Collection = ""
			So(imp.validateSettings(), ShouldBeNil)
			So(imp.ToolOptions.Collection, ShouldEqual, "input")
		})

		Convey("an error should be thrown with a system. collection", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.File = "input"
			imp.InputOptions.HeaderLine = true
			imp.InputOptions.Type = CSV
			imp.ToolOptions.Collection = common.TimeseriesBucketPrefix + "foo"
			So(imp.validateSettings(), ShouldNotBeNil)
		})

		Convey(
			"error should be thrown if --legacy is specified and input type is not JSON",
			func() {
				imp := NewMockMongoImport()
				imp.InputOptions.Type = CSV
				fieldFile := "test.csv"
				imp.InputOptions.FieldFile = &fieldFile
				imp.InputOptions.Legacy = true
				So(imp.validateSettings(), ShouldNotBeNil)
			},
		)
	})
}

func TestGetSourceReader(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Given a mongoimport instance, on calling getSourceReader", t,
		func() {
			Convey("an error should be thrown if the given file referenced by "+
				"the reader does not exist", func() {
				imp := NewMockMongoImport()
				imp.InputOptions.File = "/path/to/input/file/dot/input.txt"
				imp.InputOptions.Type = CSV
				imp.ToolOptions.Collection = ""
				_, _, err := imp.getSourceReader()
				So(err, ShouldNotBeNil)
			})

			Convey("no error should be thrown if the file exists", func() {
				imp := NewMockMongoImport()
				imp.InputOptions.File = "testdata/test_array.json"
				imp.InputOptions.Type = JSON
				_, _, err := imp.getSourceReader()
				So(err, ShouldBeNil)
			})

			Convey("no error should be thrown if stdin is used", func() {
				imp := NewMockMongoImport()
				imp.InputOptions.File = ""
				_, _, err := imp.getSourceReader()
				So(err, ShouldBeNil)
			})
		})
}

func TestGetInputReader(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("Given a io.Reader on calling getInputReader", t, func() {
		Convey("should parse --fields using valid csv escaping", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Fields = new(string)
			*imp.InputOptions.Fields = "foo.auto(),bar.date(January 2, 2006)"
			imp.InputOptions.File = "/path/to/input/file/dot/input.txt"
			imp.InputOptions.ColumnsHaveTypes = true
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("should complain about non-escaped new lines in --fields", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Fields = new(string)
			*imp.InputOptions.Fields = "foo.auto(),\nblah.binary(hex),bar.date(January 2, 2006)"
			imp.InputOptions.File = "/path/to/input/file/dot/input.txt"
			imp.InputOptions.ColumnsHaveTypes = true
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("no error should be thrown if neither --fields nor --fieldFile "+
			"is used", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.File = "/path/to/input/file/dot/input.txt"
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("no error should be thrown if --fields is used", func() {
			imp := NewMockMongoImport()
			fields := "a,b,c"
			imp.InputOptions.Fields = &fields
			imp.InputOptions.File = "/path/to/input/file/dot/input.txt"
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("no error should be thrown if --fieldFile is used and it "+
			"references a valid file", func() {
			imp := NewMockMongoImport()
			fieldFile := "testdata/test.csv"
			imp.InputOptions.FieldFile = &fieldFile
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("an error should be thrown if --fieldFile is used and it "+
			"references an invalid file", func() {
			imp := NewMockMongoImport()
			fieldFile := "/path/to/input/file/dot/input.txt"
			imp.InputOptions.FieldFile = &fieldFile
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldNotBeNil)
		})
		Convey("no error should be thrown for CSV import inputs", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Type = CSV
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("no error should be thrown for TSV import inputs", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Type = TSV
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("no error should be thrown for JSON import inputs", func() {
			imp := NewMockMongoImport()
			imp.InputOptions.Type = JSON
			_, err := imp.getInputReader(&os.File{})
			So(err, ShouldBeNil)
		})
		Convey("an error should be thrown if --fieldFile fields are invalid", func() {
			imp := NewMockMongoImport()
			fieldFile := "testdata/test_fields_invalid.txt"
			imp.InputOptions.FieldFile = &fieldFile
			file, err := os.Open(fieldFile)
			So(err, ShouldBeNil)
			_, err = imp.getInputReader(file)
			So(err, ShouldNotBeNil)
		})
		Convey("no error should be thrown if --fieldFile fields are valid", func() {
			imp := NewMockMongoImport()
			fieldFile := "testdata/test_fields_valid.txt"
			imp.InputOptions.FieldFile = &fieldFile
			file, err := os.Open(fieldFile)
			So(err, ShouldBeNil)
			_, err = imp.getInputReader(file)
			So(err, ShouldBeNil)
		})
	})
}

func TestImportDocuments(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	Convey("With a mongoimport instance", t, func() {
		Reset(func() {
			sessionProvider, err := db.NewSessionProvider(*getBasicToolOptions())
			if err != nil {
				t.Fatalf("error getting session provider session: %v", err)
			}
			session, err := sessionProvider.GetSession()
			if err != nil {
				t.Fatalf("error getting session: %v", err)
			}
			_, err = session.Database(testDb).
				Collection(testCollection).
				DeleteMany(t.Context(), bson.D{})
			if err != nil {
				t.Fatalf("error dropping collection: %v", err)
			}
		})
		Convey("no error should be thrown for CSV import on test data and all "+
			"CSV data lines should be imported correctly", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "a,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.WriteConcern = "majority"
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)
		})
		Convey("an error should be thrown for JSON import on test data that is "+
			"JSON array", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.File = "testdata/test_array.json"
			imp.IngestOptions.WriteConcern = "majority"
			numProcessed, _, err := imp.ImportDocuments()
			So(err, ShouldNotBeNil)
			So(numProcessed, ShouldEqual, 0)
		})
		Convey("TOOLS-247: no error should be thrown for JSON import on test "+
			"data and all documents should be imported correctly", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.File = "testdata/test_plain2.json"
			imp.IngestOptions.WriteConcern = "majority"
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 10)
			So(numFailed, ShouldEqual, 0)
		})
		Convey("CSV import with --ignoreBlanks should import only non-blank fields", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_blanks.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.IgnoreBlanks = true

			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2)},
				{"_id": int32(5), "c": "6e"},
				{"_id": int32(7), "b": int32(8), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("CSV import without --ignoreBlanks should include blanks", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_blanks.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(numFailed, ShouldEqual, 0)
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2), "c": ""},
				{"_id": int32(5), "b": "", "c": "6e"},
				{"_id": int32(7), "b": int32(8), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("no error should be thrown for CSV import on test data with --upsertFields", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.UpsertFields = "b,c"
			imp.IngestOptions.MaintainInsertionOrder = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(numFailed, ShouldEqual, 0)
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2), "c": int32(3)},
				{"_id": int32(3), "b": 5.4, "c": "string"},
				{"_id": int32(5), "b": int32(6), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("no error should be thrown for CSV import on test data with "+
			"--stopOnError. Only documents before error should be imported", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.StopOnError = true
			imp.IngestOptions.MaintainInsertionOrder = true
			imp.IngestOptions.WriteConcern = "majority"
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2), "c": int32(3)},
				{"_id": int32(3), "b": 5.4, "c": "string"},
				{"_id": int32(5), "b": int32(6), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey(
			"CSV import with duplicate _id's should not error if --stopOnError is not set",
			func() {
				imp, err := NewMongoImport()
				So(err, ShouldBeNil)
				imp.IngestOptions.Mode = modeInsert
				imp.InputOptions.Type = CSV
				imp.InputOptions.File = "testdata/test_duplicate.csv"
				fields := "_id,b,c"
				imp.InputOptions.Fields = &fields
				imp.IngestOptions.StopOnError = false
				numProcessed, numFailed, err := imp.ImportDocuments()
				So(err, ShouldBeNil)
				So(numProcessed, ShouldEqual, 4)
				So(numFailed, ShouldEqual, 1)

				expectedDocuments := []bson.M{
					{"_id": int32(1), "b": int32(2), "c": int32(3)},
					{"_id": int32(3), "b": 5.4, "c": "string"},
					{"_id": int32(5), "b": int32(6), "c": int32(6)},
					{"_id": int32(8), "b": int32(6), "c": int32(6)},
				}
				// all docs except the one with duplicate _id - should be imported
				So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
			},
		)
		Convey("no error should be thrown for CSV import on test data with --drop", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.Drop = true
			imp.IngestOptions.MaintainInsertionOrder = true
			imp.IngestOptions.WriteConcern = "majority"
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(numFailed, ShouldEqual, 0)
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2), "c": int32(3)},
				{"_id": int32(3), "b": 5.4, "c": "string"},
				{"_id": int32(5), "b": int32(6), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("CSV import on test data with --headerLine should succeed", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.InputOptions.HeaderLine = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 2)
			So(numFailed, ShouldEqual, 0)
		})
		Convey("EOF should be thrown for CSV import with --headerLine if file is empty", func() {
			csvFile, err := os.CreateTemp("", "mongoimport_")
			So(err, ShouldBeNil)
			csvFile.Close()

			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = csvFile.Name()
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.InputOptions.HeaderLine = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldEqual, io.EOF)
			So(numProcessed, ShouldEqual, 0)
			So(numFailed, ShouldEqual, 0)
		})
		Convey("CSV import with --mode=upsert and --upsertFields should succeed", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.UpsertFields = "_id"
			imp.IngestOptions.MaintainInsertionOrder = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "c": int32(2), "b": int32(3)},
				{"_id": int32(3), "c": 5.4, "b": "string"},
				{"_id": int32(5), "c": int32(6), "b": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("CSV import with --mode=delete should succeed", func() {
			// First import 3 documents
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.MaintainInsertionOrder = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)

			// Then delete two documents
			imp, err = NewMongoImport()
			So(err, ShouldBeNil)

			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_delete.csv"
			fields = "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.Mode = modeDelete
			imp.IngestOptions.StopOnError = true
			// Must specify upsert fields since option parsing is skipped in tests
			imp.upsertFields = []string{"_id"}
			numProcessed, numFailed, err = imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 2)
			So(numFailed, ShouldEqual, 0)

			expectedDocuments := []bson.M{
				{"_id": int32(3), "c": 5.4, "b": "string"},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("CSV import with --mode=delete and --upsertFields should succeed", func() {
			// First import 3 documents
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.MaintainInsertionOrder = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)

			// Then delete two documents
			imp, err = NewMongoImport()
			So(err, ShouldBeNil)

			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_delete.csv"
			fields = "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.Mode = modeDelete
			imp.IngestOptions.StopOnError = true
			// Must specify upsert fields since option parsing is skipped in tests
			imp.upsertFields = []string{"b", "c"}
			numProcessed, numFailed, err = imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 1)
			So(numFailed, ShouldEqual, 0)

			expectedDocuments := []bson.M{
				{"_id": int32(3), "c": 5.4, "b": "string"},
				{"_id": int32(5), "c": int32(6), "b": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("CSV import with --mode=delete and --ignoreBlanks should not take any action for "+
			"documents that have blank values for upsert fields", func() {
			// First import 3 documents
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test.csv"
			fields := "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.MaintainInsertionOrder = true
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 0)

			// Then delete two documents
			imp, err = NewMongoImport()
			So(err, ShouldBeNil)

			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_delete_with_blanks.csv"
			fields = "_id,c,b"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.Mode = modeDelete
			imp.IngestOptions.IgnoreBlanks = true
			imp.IngestOptions.StopOnError = true
			// Must specify upsert fields since option parsing is skipped in tests
			imp.upsertFields = []string{"c"}
			numProcessed, numFailed, err = imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 1)
			So(numFailed, ShouldEqual, 0)

			expectedDocuments := []bson.M{
				{"_id": int32(3), "c": 5.4, "b": "string"},
				{"_id": int32(5), "c": int32(6), "b": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("CSV import with --mode=upsert/--upsertFields with duplicate id should succeed "+
			"even if stopOnError is set", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_duplicate.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.Mode = modeUpsert
			imp.IngestOptions.StopOnError = true
			imp.upsertFields = []string{"_id"}
			numProcessed, numFailed, err := imp.ImportDocuments()
			So(err, ShouldBeNil)
			So(numProcessed, ShouldEqual, 5)
			So(numFailed, ShouldEqual, 0)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2), "c": int32(3)},
				{"_id": int32(3), "b": 5.4, "c": "string"},
				{"_id": int32(5), "b": int32(6), "c": int32(9)},
				{"_id": int32(8), "b": int32(6), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("an error should be thrown for CSV import on test data with "+
			"duplicate _id if --stopOnError is set", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_duplicate.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.StopOnError = true
			imp.IngestOptions.WriteConcern = "1"
			imp.IngestOptions.MaintainInsertionOrder = true
			numInserted, numFailed, err := imp.ImportDocuments()
			So(err, ShouldNotBeNil)
			So(numInserted, ShouldEqual, 3)
			So(numFailed, ShouldEqual, 1)
			expectedDocuments := []bson.M{
				{"_id": int32(1), "b": int32(2), "c": int32(3)},
				{"_id": int32(3), "b": 5.4, "c": "string"},
				{"_id": int32(5), "b": int32(6), "c": int32(6)},
			}
			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		})
		Convey("an error should be thrown for JSON import on test data that "+
			"is a JSON array without passing --jsonArray", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.File = "testdata/test_array.json"
			imp.IngestOptions.WriteConcern = "1"
			numInserted, _, err := imp.ImportDocuments()
			So(err, ShouldNotBeNil)
			So(numInserted, ShouldEqual, 0)
		})
		Convey("an error should be thrown if a plain JSON file is supplied", func() {
			fileHandle, err := os.Open("testdata/test_plain.json")
			So(err, ShouldBeNil)
			jsonInputReader := NewJSONInputReader(true, true, fileHandle, 1)
			docChan := make(chan bson.D, 1)
			So(jsonInputReader.StreamDocument(t.Context(), true, docChan), ShouldNotBeNil)
		})
		Convey("an error should be thrown for invalid CSV import on test data", func() {
			imp, err := NewMongoImport()
			So(err, ShouldBeNil)
			imp.IngestOptions.Mode = modeInsert
			imp.InputOptions.Type = CSV
			imp.InputOptions.File = "testdata/test_bad.csv"
			fields := "_id,b,c"
			imp.InputOptions.Fields = &fields
			imp.IngestOptions.StopOnError = true
			imp.IngestOptions.WriteConcern = "1"
			imp.IngestOptions.MaintainInsertionOrder = true
			imp.IngestOptions.BulkBufferSize = 1
			_, _, err = imp.ImportDocuments()
			So(err, ShouldNotBeNil)
		})
		Convey(
			"CSV import with --mode=upsert/--upsertFields with a nested upsert field should succeed when repeated",
			func() {
				imp, err := NewMongoImport()
				So(err, ShouldBeNil)
				imp.InputOptions.Type = CSV
				imp.InputOptions.File = "testdata/test_nested_upsert.csv"
				imp.InputOptions.HeaderLine = true
				imp.IngestOptions.Mode = modeUpsert
				imp.upsertFields = []string{"level1.level2.key1"}
				numProcessed, numFailed, err := imp.ImportDocuments()
				So(err, ShouldBeNil)
				So(numProcessed, ShouldEqual, 1)
				So(numFailed, ShouldEqual, 0)
				n, err := countDocuments(t, imp.SessionProvider)
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 1)

				// Repeat must succeed
				imp, err = NewMongoImport()
				So(err, ShouldBeNil)
				imp.InputOptions.Type = CSV
				imp.InputOptions.File = "testdata/test_nested_upsert.csv"
				imp.InputOptions.HeaderLine = true
				imp.IngestOptions.Mode = modeUpsert
				imp.upsertFields = []string{"level1.level2.key1"}
				numProcessed, numFailed, err = imp.ImportDocuments()
				So(err, ShouldBeNil)
				So(numProcessed, ShouldEqual, 1)
				So(numFailed, ShouldEqual, 0)
				n, err = countDocuments(t, imp.SessionProvider)
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 1)
			},
		)
		Convey("With --useArrayIndexFields: Top-level numerical fields should be document keys",
			nestedFieldsTestHelper(
				t,
				"_id,0,1\n1,2,3",
				[]bson.M{
					{"_id": int32(1), "0": int32(2), "1": int32(3)},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Should insert nested document",
			nestedFieldsTestHelper(
				t,
				"_id,a.a,a.b\n1,2,3",
				[]bson.M{
					{"_id": int32(1), "a": bson.M{"a": int32(2), "b": int32(3)}},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Should insert an array",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.1\n1,2,3",
				[]bson.M{
					{"_id": int32(1), "a": bson.A{int32(2), int32(3)}},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Should insert an array of documents",
			nestedFieldsTestHelper(
				t,
				"_id,a.0.a,a.0.b,a.1.a\n1,2,3,4",
				[]bson.M{
					{
						"_id": int32(1),
						"a":   bson.A{bson.M{"a": int32(2), "b": int32(3)}, bson.M{"a": int32(4)}},
					},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Should insert an array of arrays",
			nestedFieldsTestHelper(
				t,
				"_id,a.0.0,a.0.1,a.1.0\n1,2,3,4",
				[]bson.M{
					{"_id": int32(1), "a": bson.A{bson.A{int32(2), int32(3)}, bson.A{int32(4)}}},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Should insert an array when top-level key is \"0\"",
			nestedFieldsTestHelper(
				t,
				"_id,0.0,0.1\n1,2,3",
				[]bson.M{
					{"_id": int32(1), "0": bson.A{int32(2), int32(3)}},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Should insert an array in a document in an array",
			nestedFieldsTestHelper(
				t,
				"_id,a.0.a.0,a.0.a.1\n1,2,3",
				[]bson.M{
					{"_id": int32(1), "a": bson.A{bson.M{"a": bson.A{int32(2), int32(3)}}}},
				},
				nil,
			),
		)
		Convey(
			"With --useArrayIndexFields: If an array element is blank in the csv file, an empty string should be inserted",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.1,a.2\n1,2,,4",
				[]bson.M{
					{"_id": int32(1), "a": bson.A{int32(2), "", int32(4)}},
				},
				nil,
			),
		)
		Convey(
			"With --useArrayIndexFields: If an array with more than 10 fields should be inserted",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.1,a.2,a.3,a.4,a.5,a.6,a.7,a.8,a.9,a.10\n0,1,2,3,4,5,6,7,8,9,10",
				[]bson.M{
					{
						"_id": int32(0),
						"a": bson.A{
							int32(1),
							int32(2),
							int32(3),
							int32(4),
							int32(5),
							int32(6),
							int32(7),
							int32(8),
							int32(9),
							int32(10),
						},
					},
				},
				nil,
			),
		)
		Convey(
			"With --useArrayIndexFields: An number with leading zeros should be interpreted as a document key, not an index",
			nestedFieldsTestHelper(
				t,
				"_id,a.0001\n1,2",
				[]bson.M{
					{"_id": int32(1), "a": bson.M{"0001": int32(2)}},
				},
				nil,
			),
		)
		Convey(
			"With --useArrayIndexFields: An number with leading plus should be interpreted as a document key, not an index",
			nestedFieldsTestHelper(
				t,
				"_id,a.+15558675309\n1,2",
				[]bson.M{
					{"_id": int32(1), "a": bson.M{"+15558675309": int32(2)}},
				},
				nil,
			),
		)
		Convey(
			"With --useArrayIndexFields: Should be able to make changes to document in an array once document has been created",
			nestedFieldsTestHelper(
				t,
				"_id,a.0.a,a.1.a,a.0.b\n1,2,3,4",
				[]bson.M{
					{
						"_id": int32(1),
						"a":   bson.A{bson.M{"a": int32(2), "b": int32(4)}, bson.M{"a": int32(3)}},
					},
				},
				nil,
			),
		)
		Convey("With --useArrayIndexFields: Duplicate fields should throw an error",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.0\n1,2,3",
				nil,
				fmt.Errorf(
					"array index error with field `a.0`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey("With --useArrayIndexFields: Array fields not starting at 0 should throw an error",
			nestedFieldsTestHelper(
				t,
				"_id,a.1,a.0\n1,2,3",
				nil,
				fmt.Errorf(
					"array index error with field `a.1`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey("With --useArrayIndexFields: Array fields skipping an index should throw an error",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.2\n1,2,3",
				nil,
				fmt.Errorf(
					"array index error with field `a.2`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey(
			"With --useArrayIndexFields: Array fields with sub documents skipping an index should throw an error",
			nestedFieldsTestHelper(
				t,
				"_id,a.0.a,a.2.a\n1,2,3",
				nil,
				fmt.Errorf(
					"array index error with field `a.2.a`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey(
			"With --useArrayIndexFields: Array field should throw an error if value has already been set as document",
			nestedFieldsTestHelper(
				t,
				"_id,a.a,a.0\n1,2,3",
				nil,
				fmt.Errorf("fields `a.a` and `a.0` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Array field should throw an error if value has already been set as document (deep object)",
			nestedFieldsTestHelper(
				t,
				"_id,a.a.a.a,a.a.0.a\n1,2,3",
				nil,
				fmt.Errorf("fields `a.a.a.a` and `a.a.0.a` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Document field should throw an error if value has already been set as array",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.a\n1,2,3",
				nil,
				fmt.Errorf("fields `a.0` and `a.a` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Document field should throw an error if value has already been set as array (deep object)",
			nestedFieldsTestHelper(
				t,
				"_id,a.a.a.0,a.a.a.a\n1,2,3",
				nil,
				fmt.Errorf("fields `a.a.a.0` and `a.a.a.a` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Array field should throw an error if value has already been set as value",
			nestedFieldsTestHelper(
				t,
				"_id,a,a.0\n1,2,3",
				nil,
				fmt.Errorf("fields `a` and `a.0` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Array field should throw an error if value has already been set as value (deep object)",
			nestedFieldsTestHelper(
				t,
				"_id,a.a.a,a.a.a.0\n1,2,3",
				nil,
				fmt.Errorf("fields `a.a.a` and `a.a.a.0` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Array field should be incompatible with a document field starting with a symbol that is sorted before 0",
			nestedFieldsTestHelper(
				t,
				"_id,a./,a.0\n1,2,3",
				nil,
				fmt.Errorf("fields `a./` and `a.0` are incompatible"),
			),
		)
		Convey("With --useArrayIndexFields: Indexes in fields must start from 0",
			nestedFieldsTestHelper(
				t,
				"_id,a,b.1\n1,2,3",
				nil,
				fmt.Errorf(
					"array index error with field `b.1`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey(
			"With --useArrayIndexFields: Indexes in fields must start from 0 (last field same length)",
			nestedFieldsTestHelper(
				t,
				"_id,a.b,b.1\n1,2,3",
				nil,
				fmt.Errorf(
					"array index error with field `b.1`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey(
			"With --useArrayIndexFields: Fields that are the same should throw an error (no arrays)",
			nestedFieldsTestHelper(
				t,
				"_id,a.b,a.b\n1,2,3",
				nil,
				fmt.Errorf("fields cannot be identical: `a.b` and `a.b`"),
			),
		)
		Convey("With --useArrayIndexFields: Repeated array index should throw error",
			nestedFieldsTestHelper(
				t,
				"_id,a.0,a.1,a.2,a.0\n1,2,3,4,5",
				nil,
				fmt.Errorf(
					"array index error with field `a.0`: array indexes in fields must start from 0 and increase sequentially",
				),
			),
		)
		Convey("With --useArrayIndexFields: Array entries of different types should throw an error",
			nestedFieldsTestHelper(
				t,
				"_id,a.a.0.a,a.a.0.1\n1,2,3",
				nil,
				fmt.Errorf("fields `a.a.0.a` and `a.a.0.1` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Document field should throw an error if element has already been set to an array",
			nestedFieldsTestHelper(
				t,
				"_id,a.0.0,a.0.a\n1,2,3",
				nil,
				fmt.Errorf("fields `a.0.0` and `a.0.a` are incompatible"),
			),
		)
		Convey(
			"With --useArrayIndexFields: Incompatible fields should throw error (one long, one short)",
			nestedFieldsTestHelper(
				t,
				"_id,a.a.a.a,a.a\n1,2,3",
				nil,
				fmt.Errorf("fields `a.a.a.a` and `a.a` are incompatible"),
			),
		)
	})
}

func nestedFieldsTestHelper(
	t *testing.T,
	data string,
	expectedDocuments []bson.M,
	expectedErr error,
) func() {
	return func() {
		err := os.WriteFile(util.ToUniversalPath("./temp_test_data.csv"), []byte(data), 0644)
		So(err, ShouldBeNil)
		defer func() {
			err = os.Remove(util.ToUniversalPath("./temp_test_data.csv"))
			So(err, ShouldBeNil)
		}()

		imp, err := NewMongoImport()
		So(err, ShouldBeNil)

		imp.InputOptions.Type = CSV
		imp.InputOptions.File = "./temp_test_data.csv"
		imp.InputOptions.HeaderLine = true
		imp.InputOptions.UseArrayIndexFields = true
		imp.IngestOptions.Mode = modeInsert

		numImported, numFailed, err := imp.ImportDocuments()
		if expectedDocuments == nil {
			So(err, ShouldNotBeNil)
			So(err, ShouldResemble, expectedErr)
		} else {
			So(err, ShouldBeNil)
			So(numImported, ShouldEqual, len(expectedDocuments))
			So(numFailed, ShouldEqual, 0)

			So(checkOnlyHasDocuments(t, imp.SessionProvider, expectedDocuments), ShouldBeNil)
		}
	}
}

// Regression test for TOOLS-1694 to prevent issue from TOOLS-1115.
func TestHiddenOptionsDefaults(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	Convey("With a new mongoimport with empty options", t, func() {
		imp := NewMockMongoImport()
		imp.ToolOptions = options.New("", "", "", "", true, options.EnabledOptions{})
		Convey("Then parsing should fill args with expected defaults", func() {
			_, err := imp.ToolOptions.ParseArgs([]string{})
			So(err, ShouldBeNil)

			// collection cannot be empty in validate
			imp.ToolOptions.Collection = "col"
			So(imp.validateSettings(), ShouldBeNil)
			So(imp.IngestOptions.NumDecodingWorkers, ShouldEqual, runtime.NumCPU())
			So(imp.IngestOptions.BulkBufferSize, ShouldEqual, 1000)
		})
	})
}

// generateTestData creates the files used in TestImportMIOSOE.
func generateTestData() error {
	// If file exists already, don't both regenerating it.
	if _, err := os.Stat(mioSoeFile); err == nil {
		return nil
	}

	f, err := os.Create(mioSoeFile)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)

	// 10k unique _id's
	for i := 1; i < 10001; i++ {
		_, err = fmt.Fprintf(w, "{\"_id\": %v }\n", i)
		if err != nil {
			return err
		}
	}
	// 1 duplicate _id
	_, err = fmt.Fprintf(w, "{\"_id\": %v }\n", 5)
	if err != nil {
		return err
	}

	// 10k unique _id's
	for i := 10001; i < 20001; i++ {
		_, err = fmt.Fprintf(w, "{\"_id\": %v }\n", i)
		if err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}

// test --maintainInsertionOrder and --stopOnError behavior.
func TestImportMIOSOE(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	if err := generateTestData(); err != nil {
		t.Fatalf("Could not generate test data: %v", err)
	}

	client, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available?? (%v)", err)
	}
	database := client.Database("miodb")
	coll := database.Collection("mio")

	Convey("default restore ignores dup key errors", t, func() {
		imp, err := getImportWithArgs(mioSoeFile,
			"--collection", coll.Name(),
			"--db", database.Name(),
			"--drop")
		So(err, ShouldBeNil)
		So(imp.IngestOptions.MaintainInsertionOrder, ShouldBeFalse)

		nSuccess, nFailure, err := imp.ImportDocuments()
		So(err, ShouldBeNil)

		So(nSuccess, ShouldEqual, 20000)
		So(nFailure, ShouldEqual, 1)

		count, err := coll.CountDocuments(t.Context(), bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 20000)
	})

	Convey("--maintainInsertionOrder stops exactly on dup key errors", t, func() {
		imp, err := getImportWithArgs(mioSoeFile,
			"--collection", coll.Name(),
			"--db", database.Name(),
			"--drop",
			"--maintainInsertionOrder")
		So(err, ShouldBeNil)
		So(imp.IngestOptions.MaintainInsertionOrder, ShouldBeTrue)
		So(imp.IngestOptions.NumInsertionWorkers, ShouldEqual, 1)

		nSuccess, nFailure, err := imp.ImportDocuments()
		So(err, ShouldNotBeNil)

		So(nSuccess, ShouldEqual, 10000)
		So(nFailure, ShouldEqual, 1)
		So(err, ShouldNotBeNil)

		count, err := coll.CountDocuments(t.Context(), bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 10000)
	})

	Convey("--stopOnError stops on dup key errors", t, func() {
		imp, err := getImportWithArgs(mioSoeFile,
			"--collection", coll.Name(),
			"--db", database.Name(),
			"--drop",
			"--stopOnError")
		So(err, ShouldBeNil)
		So(imp.IngestOptions.StopOnError, ShouldBeTrue)

		nSuccess, nFailure, err := imp.ImportDocuments()
		So(err, ShouldNotBeNil)

		So(nSuccess, ShouldAlmostEqual, 10000, imp.IngestOptions.BulkBufferSize)
		So(nFailure, ShouldEqual, 1)

		count, err := coll.CountDocuments(t.Context(), bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldAlmostEqual, 10000, imp.IngestOptions.BulkBufferSize)
	})

	_ = database.Drop(t.Context())
}

// TestImportBooleanType verifies that mongoimport correctly imports legacy
// JSON with Boolean() constructor syntax using --legacy.
func TestImportBooleanType(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_booleantype_test"
	const collName = "testcollbool"

	client := newImportTestClient(t, dbName)

	entries := []struct{ key, expr string }{
		{"a", "Boolean(1)"},
		{"b", "Boolean(0)"},
		{"c", "Boolean(140)"},
		{"d", "Boolean(-140.5)"},
		{"e", "Boolean(Boolean(1))"},
		{"f", "Boolean(Boolean(0))"},
		{"g", "Boolean('')"},
		{"h", "Boolean('f')"},
		{"i", "Boolean(null)"},
		{"j", "Boolean(undefined)"},
		{"k", "Boolean(true)"},
		{"l", "Boolean(false)"},
		{"m", "Boolean(true, false)"},
		{"n", "Boolean(false, true)"},
		{"o", "[ Boolean(1), Boolean(0), Date(23) ]"},
		{"p", "Boolean(Date(15))"},
		{"q", "Boolean(0x585)"},
		{"r", "Boolean(0x0)"},
		{"s", "Boolean()"},
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "boolean-*.json")
	require.NoError(t, err)
	for _, e := range entries {
		_, err = fmt.Fprintf(tmpFile, "{ key: '%s', bool: %s }\n", e.key, e.expr)
		require.NoError(t, err)
	}
	require.NoError(t, tmpFile.Close())

	importToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	importToolOptions.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := New(Options{
		ToolOptions: importToolOptions,
		InputOptions: &InputOptions{
			File:       tmpFile.Name(),
			ParseGrace: "stop",
			Legacy:     true,
		},
		IngestOptions: &IngestOptions{},
	})
	require.NoError(t, err)
	imported, _, err := mi.ImportDocuments()
	require.NoError(t, err)
	assert.EqualValues(t, len(entries), imported, "should import all documents")

	coll := client.Database(dbName).Collection(collName)
	for _, e := range entries {
		n, err := coll.CountDocuments(t.Context(), bson.D{{"key", e.key}})
		require.NoError(t, err)
		assert.EqualValues(t, 1, n, "document with key=%q should exist", e.key)
	}
}

// TestImportCollectionNameDerivation verifies that mongoimport correctly
// derives the collection name from the input filename.
func TestImportCollectionNameDerivation(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_collections_test"

	client := newImportTestClient(t, dbName)
	t.Cleanup(func() {
		_ = client.Database("testdb2").Drop(t.Context())
	})

	tmpDir := t.TempDir()
	rows := []map[string]int{{"a": 1, "b": 2, "c": 3}, {"a": 4, "b": 5, "c": 6}}
	var sb strings.Builder
	for _, row := range rows {
		b, err := json.Marshal(row)
		require.NoError(t, err)
		sb.Write(b)
		sb.WriteByte('\n')
	}

	fooBlahJSON := writeTestFile(t, tmpDir, "foo.blah.json", sb.String())
	importFromFile(t, fooBlahJSON, dbName, "")
	assertImported(t, client, dbName, "foo.blah")

	fooBlahJSONBackup := writeTestFile(t, tmpDir, "foo.blah.json.backup", sb.String())
	importFromFile(t, fooBlahJSONBackup, dbName, "")
	assertImported(t, client, dbName, "foo.blah.json")

	importFromFile(t, fooBlahJSON, dbName, "testcoll1")
	assertImported(t, client, dbName, "testcoll1")

	importFromFile(t, fooBlahJSON, "testdb2", "")
	assertImported(t, client, "testdb2", "foo.blah")
}

// TestImportFields verifies --headerline, --fields, --fieldFile, --ignoreBlanks,
// nested dotted field names, and extra-fields-beyond-header behavior.
func TestImportFields(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_fields_test"

	client := newImportTestClient(t, dbName)

	tmpDir := t.TempDir()
	header := []string{"a", "b", "c.xyz", "d.hij.lkm"}
	rows := [][]string{
		{"foo", "bar", "blah", "qwz"},
		{"bob", "", "steve", "sue"},
		{"one", "two", "three", "four"},
	}

	for _, format := range []string{"csv", "tsv"} {
		testImportFieldsForFormat(t, client, dbName, tmpDir, format, header, rows)
	}
}

// TestImportExtraFields verifies that CSV columns beyond the fieldFile mapping
// are imported as field4, field5, etc.
func TestImportExtraFields(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const dbName = "mongoimport_extrafields_test"
	const collName = "extrafields"

	client := newImportTestClient(t, dbName)

	tmpDir := t.TempDir()
	extraRows := [][]string{
		{"foo", "bar", "blah", "qwz"},
		{"bob", "", "steve", "sue"},
		{"one", "two", "three", "four", "extra1", "extra2", "extra3"},
	}
	extraFile := filepath.Join(tmpDir, "extrafields.csv")
	writeXSVFile(t, extraFile, ',', extraRows)

	fieldNames := []string{"a", "b", "c.xyz", "d.hij.lkm"}
	fieldFilePath := writeFieldFile(t, tmpDir, "fieldfile", fieldNames)

	toolOpts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOpts.Namespace = &options.Namespace{DB: dbName, Collection: collName}
	mi, err := New(Options{
		ToolOptions: toolOpts,
		InputOptions: &InputOptions{
			File:       extraFile,
			Type:       "csv",
			FieldFile:  &fieldFilePath,
			ParseGrace: "stop",
		},
		IngestOptions: &IngestOptions{},
	})
	require.NoError(t, err)
	_, _, err = mi.ImportDocuments()
	require.NoError(t, err)

	var doc bson.M
	err = client.Database(dbName).Collection(collName).
		FindOne(t.Context(), bson.D{{"a", "one"}}).Decode(&doc)
	require.NoError(t, err)
	assert.Equal(t, "extra1", doc["field4"], "field4 should contain first extra value")
	assert.Equal(t, "extra2", doc["field5"], "field5 should contain second extra value")
	assert.Equal(t, "extra3", doc["field6"], "field6 should contain third extra value")
}

// TestImportModeUpsertFields tests --mode with --upsertFields a,c (compound key matching).
func TestImportModeUpsertFields(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongoimport_modes_upsertfields_test"
		collName = "c"
	)

	client := newImportTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	ns := &options.Namespace{DB: dbName, Collection: collName}
	dir := t.TempDir()

	importFile := writeJSONLinesFile(t, dir, "upsert2.json", []map[string]any{
		{"a": 1234, "b": 4567, "c": 222},
		{"a": 4567, "b": "yyy", "c": 333},
		{"a": 1234, "b": "blah", "c": 222},
		{"a": "xxx", "b": "test", "c": -1},
		{"a": 4567, "b": "asdf", "c": 222},
	})

	t.Run("mode=wrong returns error", func(t *testing.T) {
		setupUpsertFieldsDocs(t, coll)
		err := importCollection(
			t,
			ns,
			importFile,
			IngestOptions{Mode: "wrong", UpsertFields: "a,c"},
		)
		require.ErrorContains(t, err, "invalid --mode argument")
	})

	t.Run("mode=insert returns error", func(t *testing.T) {
		setupUpsertFieldsDocs(t, coll)
		err := importCollection(
			t,
			ns,
			importFile,
			IngestOptions{Mode: modeInsert, UpsertFields: "a,c"},
		)
		require.ErrorContains(t, err, "cannot use --upsertFields with --mode=insert")
	})

	for _, tc := range []struct {
		name string
		opts IngestOptions
	}{
		{"default mode", IngestOptions{UpsertFields: "a,c"}},
		{"deprecated --upsert", IngestOptions{Upsert: true, UpsertFields: "a,c"}},
		{"mode=upsert", IngestOptions{Mode: modeUpsert, UpsertFields: "a,c"}},
	} {
		t.Run(tc.name+" replaces matched docs", func(t *testing.T) {
			setupUpsertFieldsDocs(t, coll)
			require.NoError(t, importCollection(t, ns, importFile, tc.opts))

			var doc map[string]any
			require.NoError(t, coll.FindOne(t.Context(), bson.D{{"b", "blah"}}).Decode(&doc))
			assert.Nil(t, doc["x"], "upsert replaces doc; original x field gone")

			doc = map[string]any{}
			require.NoError(t, coll.FindOne(t.Context(), bson.D{{"b", "yyy"}}).Decode(&doc))
			assert.Nil(t, doc["x"], "upsert replaces doc; original x field gone")

			doc = map[string]any{}
			require.NoError(t, coll.FindOne(t.Context(), bson.D{{"b", "asdf"}}).Decode(&doc))
			assert.Nil(t, doc["x"], "newly inserted doc has no x field")
		})
	}

	t.Run("mode=merge updates matched docs and preserves unset fields", func(t *testing.T) {
		setupUpsertFieldsDocs(t, coll)
		require.NoError(
			t,
			importCollection(
				t,
				ns,
				importFile,
				IngestOptions{Mode: modeMerge, UpsertFields: "a,c"},
			),
		)

		var doc map[string]any
		require.NoError(t, coll.FindOne(t.Context(), bson.D{{"b", "blah"}}).Decode(&doc))
		assert.Equal(t, "original field", doc["x"], "merge preserves x on matched doc")

		doc = map[string]any{}
		require.NoError(t, coll.FindOne(t.Context(), bson.D{{"b", "yyy"}}).Decode(&doc))
		assert.Equal(t, "original field", doc["x"], "merge preserves x on matched doc")

		doc = map[string]any{}
		require.NoError(t, coll.FindOne(t.Context(), bson.D{{"b", "asdf"}}).Decode(&doc))
		assert.Nil(t, doc["x"], "newly inserted doc has no x field")
	})
}

// TestImportModeByID tests --mode without --upsertFields (matches on _id).
func TestImportModeByID(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongoimport_modes_byid_test"
		collName = "c"
	)

	client := newImportTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	ns := &options.Namespace{DB: dbName, Collection: collName}
	dir := t.TempDir()

	importFile := writeJSONLinesFile(t, dir, "upsert1.json", []map[string]any{
		{"_id": "one", "a": 1234, "b": 4567},
		{"_id": "two", "a": "xxx", "b": "yyy"},
		{"_id": "one", "a": "foo", "b": "blah"},
		{"_id": "one", "a": "test", "b": "test"},
		{"_id": "one", "a": "unicorns", "b": "zebras"},
	})

	t.Run("mode=wrong returns error", func(t *testing.T) {
		setupByIDDocs(t, coll)
		err := importCollection(t, ns, importFile, IngestOptions{Mode: "wrong"})
		require.ErrorContains(t, err, "invalid --mode argument")
	})

	for _, tc := range []struct {
		name string
		opts IngestOptions
	}{
		{"mode=insert", IngestOptions{Mode: modeInsert}},
		{"default mode", IngestOptions{}},
	} {
		t.Run(tc.name+" skips duplicates", func(t *testing.T) {
			setupByIDDocs(t, coll)
			require.NoError(t, importCollection(t, ns, importFile, tc.opts))

			n, err := coll.CountDocuments(t.Context(), bson.D{})
			require.NoError(t, err)
			assert.EqualValues(t, 2, n, "all entries were duplicates; count unchanged")

			var doc map[string]any
			require.NoError(t, coll.FindOne(t.Context(), bson.D{{"_id", "one"}}).Decode(&doc))
			assert.Equal(t, "original value", doc["a"], "_id=one unchanged")
			assert.Equal(t, "original field", doc["x"], "_id=one unchanged")
		})
	}

	for _, tc := range []struct {
		name string
		opts IngestOptions
	}{
		{"deprecated --upsert", IngestOptions{Upsert: true}},
		{"mode=upsert", IngestOptions{Mode: modeUpsert}},
	} {
		t.Run(tc.name+" replaces docs", func(t *testing.T) {
			setupByIDDocs(t, coll)
			require.NoError(t, importCollection(t, ns, importFile, tc.opts))

			var doc map[string]any
			require.NoError(t, coll.FindOne(t.Context(), bson.D{{"_id", "one"}}).Decode(&doc))
			assert.Equal(t, "unicorns", doc["a"], "last write wins")
			assert.Equal(t, "zebras", doc["b"], "last write wins")
			assert.Nil(t, doc["x"], "upsert replaces doc; original x field gone")

			doc = map[string]any{}
			require.NoError(t, coll.FindOne(t.Context(), bson.D{{"_id", "two"}}).Decode(&doc))
			assert.Equal(t, "xxx", doc["a"])
			assert.Equal(t, "yyy", doc["b"])
			assert.Nil(t, doc["x"], "upsert replaces doc; original x field gone")
		})
	}

	t.Run("mode=merge updates docs and preserves unset fields", func(t *testing.T) {
		setupByIDDocs(t, coll)
		require.NoError(t, importCollection(t, ns, importFile, IngestOptions{Mode: modeMerge}))

		var doc map[string]any
		require.NoError(t, coll.FindOne(t.Context(), bson.D{{"_id", "one"}}).Decode(&doc))
		assert.Equal(t, "unicorns", doc["a"], "last write wins")
		assert.Equal(t, "zebras", doc["b"], "last write wins")
		assert.Equal(t, "original field", doc["x"], "merge preserves x")

		doc = map[string]any{}
		require.NoError(t, coll.FindOne(t.Context(), bson.D{{"_id", "two"}}).Decode(&doc))
		assert.Equal(t, "xxx", doc["a"])
		assert.Equal(t, "yyy", doc["b"])
		assert.Equal(t, "original field", doc["x"], "merge preserves x")
	})
}

func newImportTestClient(t *testing.T, dbName string) *mongo.Client {
	t.Helper()
	ssl := testutil.GetSSLOptions()
	auth := testutil.GetAuthOptions()
	sessionProvider, err := db.NewSessionProvider(options.ToolOptions{
		General: &options.General{},
		SSL:     &ssl,
		Connection: &options.Connection{
			Host: "localhost",
			Port: db.DefaultTestPort,
		},
		Auth:         &auth,
		URI:          &options.URI{},
		Namespace:    &options.Namespace{},
		WriteConcern: wcwrapper.Majority(),
	})
	require.NoError(t, err, "should create session provider")
	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "should get session")
	t.Cleanup(func() {
		_ = client.Database(dbName).Drop(t.Context())
	})
	return client
}

func importCollection(
	t *testing.T,
	ns *options.Namespace,
	filePath string,
	ingestOpts IngestOptions,
) error {
	t.Helper()
	toolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOptions.Namespace = ns
	mi, err := New(Options{
		ToolOptions:   toolOptions,
		InputOptions:  &InputOptions{File: filePath, ParseGrace: "stop"},
		IngestOptions: &ingestOpts,
	})
	if err != nil {
		return err
	}
	defer mi.Close()
	_, _, err = mi.ImportDocuments()
	return err
}

func importFromFile(t *testing.T, filePath, dbOverride, collOverride string) {
	t.Helper()
	toolOpts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOpts.Namespace = &options.Namespace{DB: dbOverride, Collection: collOverride}
	mi, err := New(Options{
		ToolOptions:   toolOpts,
		InputOptions:  &InputOptions{File: filePath, ParseGrace: "stop"},
		IngestOptions: &IngestOptions{},
	})
	require.NoError(t, err)
	_, _, err = mi.ImportDocuments()
	require.NoError(t, err)
}

func assertImported(t *testing.T, client *mongo.Client, dbName, coll string) {
	t.Helper()
	n, err := client.Database(dbName).Collection(coll).CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "%s.%s should have 2 imported documents", dbName, coll)
	require.NoError(t, client.Database(dbName).Collection(coll).Drop(t.Context()))
}

func setupUpsertFieldsDocs(t *testing.T, coll *mongo.Collection) {
	t.Helper()
	require.NoError(t, coll.Drop(t.Context()))
	_, err := coll.InsertMany(
		t.Context(),
		[]any{
			bson.D{{"a", int32(1234)}, {"b", "000000"}, {"c", int32(222)}, {"x", "original field"}},
			bson.D{{"a", int32(4567)}, {"b", "111111"}, {"c", int32(333)}, {"x", "original field"}},
		},
	)
	require.NoError(t, err)
}

func setupByIDDocs(t *testing.T, coll *mongo.Collection) {
	t.Helper()
	require.NoError(t, coll.Drop(t.Context()))
	_, err := coll.InsertMany(
		t.Context(),
		[]any{
			bson.D{{"_id", "one"}, {"a", "original value"}, {"x", "original field"}},
			bson.D{{"_id", "two"}, {"a", "original value 2"}, {"x", "original field"}},
		},
	)
	require.NoError(t, err)
}

func testImportFieldsForFormat(
	t *testing.T,
	client *mongo.Client,
	dbName, tmpDir, format string,
	header []string,
	rows [][]string,
) {
	t.Helper()

	separator, ok := map[string]rune{
		"csv": ',',
		"tsv": '\t',
	}[format]
	require.True(t, ok, "found a separator for %#q format", format)

	headerFile := filepath.Join(tmpDir, "header."+format)
	writeXSVFile(t, headerFile, separator, append([][]string{header}, rows...))

	noHeaderFile := filepath.Join(tmpDir, "noheader."+format)
	writeXSVFile(t, noHeaderFile, separator, rows)
	fieldFilePath := writeFieldFile(t, tmpDir, "fieldfile."+format, header)

	t.Run(format+"/headerline", func(t *testing.T) {
		importAndCheckFields(t, client, importFieldsOpts{
			dbName:     dbName,
			inputFile:  headerFile,
			format:     format,
			headerLine: true,
		})
	})

	t.Run(format+"/fields", func(t *testing.T) {
		fields := "a,b,c.xyz,d.hij.lkm"
		importAndCheckFields(t, client, importFieldsOpts{
			dbName:    dbName,
			inputFile: noHeaderFile,
			format:    format,
			fields:    &fields,
		})
	})

	t.Run(format+"/fieldFile", func(t *testing.T) {
		importAndCheckFields(t, client, importFieldsOpts{
			dbName:        dbName,
			inputFile:     noHeaderFile,
			format:        format,
			fieldFilePath: fieldFilePath,
		})
		coll := client.Database(dbName).Collection(format + "testcoll")
		var bobDoc bson.M
		err := coll.FindOne(t.Context(), bson.D{{"a", "bob"}}).Decode(&bobDoc)
		require.NoError(t, err)
		assert.Equal(
			t, "", bobDoc["b"],
			"%s: blank field should be empty string without --ignoreBlanks", format,
		)
	})

	t.Run(format+"/ignoreBlanks", func(t *testing.T) {
		importAndCheckFields(t, client, importFieldsOpts{
			dbName:        dbName,
			inputFile:     noHeaderFile,
			format:        format,
			fieldFilePath: fieldFilePath,
			ignoreBlanks:  true,
		})
		coll := client.Database(dbName).Collection(format + "testcoll")
		var bobDoc bson.M
		err := coll.FindOne(t.Context(), bson.D{{"a", "bob"}}).Decode(&bobDoc)
		require.NoError(t, err)
		_, hasB := bobDoc["b"]
		assert.False(t, hasB, "%s: blank field should be omitted with --ignoreBlanks", format)
	})

	t.Run(format+"/noFieldSpec", func(t *testing.T) {
		toolOpts, err := testutil.GetToolOptions()
		require.NoError(t, err)
		toolOpts.Namespace = &options.Namespace{DB: dbName, Collection: format + "testcoll"}
		_, err = New(Options{
			ToolOptions:   toolOpts,
			InputOptions:  &InputOptions{File: noHeaderFile, Type: format, ParseGrace: "stop"},
			IngestOptions: &IngestOptions{},
		})
		assert.Error(t, err, "%s: import without field spec should fail", format)
	})
}

type importFieldsOpts struct {
	dbName        string
	inputFile     string
	format        string
	fieldFilePath string
	fields        *string
	headerLine    bool
	ignoreBlanks  bool
}

func importAndCheckFields(t *testing.T, client *mongo.Client, o importFieldsOpts) {
	t.Helper()
	collName := o.format + "testcoll"
	require.NoError(t, client.Database(o.dbName).Collection(collName).Drop(t.Context()))
	toolOpts, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOpts.Namespace = &options.Namespace{DB: o.dbName, Collection: collName}
	var ffPtr *string
	if o.fieldFilePath != "" {
		ffPtr = &o.fieldFilePath
	}
	mi, err := New(Options{
		ToolOptions: toolOpts,
		InputOptions: &InputOptions{
			File:       o.inputFile,
			Type:       o.format,
			HeaderLine: o.headerLine,
			FieldFile:  ffPtr,
			Fields:     o.fields,
			ParseGrace: "stop",
		},
		IngestOptions: &IngestOptions{IgnoreBlanks: o.ignoreBlanks},
	})
	require.NoError(t, err)
	_, _, err = mi.ImportDocuments()
	require.NoError(t, err)

	coll := client.Database(o.dbName).Collection(collName)
	n, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, n, "%s: should import 3 documents", o.format)

	nestedQuery := bson.D{
		{"a", "foo"},
		{"b", "bar"},
		{"c.xyz", "blah"},
		{"d.hij.lkm", "qwz"},
	}
	n, err = coll.CountDocuments(t.Context(), nestedQuery)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "%s: nested fields should be stored correctly", o.format)
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func writeJSONLinesFile(t *testing.T, dir, name string, docs []map[string]any) string {
	t.Helper()
	var buf bytes.Buffer
	for _, doc := range docs {
		b, err := json.Marshal(doc)
		require.NoError(t, err)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return writeTestFile(t, dir, name, buf.String())
}

func writeXSVFile(t *testing.T, path string, separator rune, records [][]string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	w := csv.NewWriter(f)
	w.Comma = separator
	require.NoError(t, w.WriteAll(records))
	require.NoError(t, f.Close())
}

func writeFieldFile(t *testing.T, dir, name string, fields []string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(fields, "\n")+"\n"), 0o644))
	return path
}
