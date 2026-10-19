// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongoimport

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/options"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitInlineHeader(t *testing.T) {
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

// Regression test for TOOLS-1694 to prevent issue from TOOLS-1115.
func TestHiddenOptionsDefaults(t *testing.T) {
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
