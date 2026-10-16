// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongodump

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoDumpValidateOptions(t *testing.T) {
	for _, testCase := range invalidDumpOptionsCases {
		t.Run(testCase.name, func(t *testing.T) {
			md, err := simpleMongoDumpInstance()
			require.NoError(t, err)

			testCase.setUp(md)

			assert.ErrorContains(
				t,
				md.ValidateOptions(),
				testCase.wantErr,
				"mongodump rejects %s",
				testCase.name,
			)
		})
	}
}

type invalidDumpOptionsCase struct {
	name    string
	setUp   func(*MongoDump)
	wantErr string
}

// invalidDumpOptionsCases collects the option combinations mongodump rejects. Each case asserts on
// the error validation returns, rather than on a process exit code, so it pins down which
// combination was rejected and why.
var invalidDumpOptionsCases = []invalidDumpOptionsCase{
	{
		name: "a collection without a db",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = ""
			md.ToolOptions.Collection = "some_collection"
		},
		wantErr: "cannot dump a collection without a specified database",
	},
	{
		name: "a query without a collection",
		setUp: func(md *MongoDump) {
			md.ToolOptions.Collection = ""
			md.InputOptions.Query = `{"_id": ""}`
		},
		wantErr: "cannot dump using a query without a specified collection",
	},
	{
		name: "dumping to stdout without a collection",
		setUp: func(md *MongoDump) {
			md.OutputOptions.Out = "-"
			md.ToolOptions.DB = "foo"
			md.ToolOptions.Collection = ""
		},
		wantErr: "can only dump a single collection to stdout",
	},
	{
		name: "queryFile without a collection",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = "foo"
			md.ToolOptions.Collection = ""
			md.InputOptions.QueryFile = "query.json"
		},
		wantErr: "cannot dump using a queryFile without a specified collection",
	},
	{
		name: "query together with queryFile",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = "foo"
			md.ToolOptions.Collection = "bar"
			md.InputOptions.Query = `{"x": 1}`
			md.InputOptions.QueryFile = "query.json"
		},
		wantErr: "either query or queryFile can be specified as a query option, not both",
	},
	{
		name: "forceTableScan together with a query",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = "foo"
			md.ToolOptions.Collection = "bar"
			md.InputOptions.Query = `{"x": 1}`
			md.InputOptions.TableScan = true
		},
		wantErr: "cannot use --forceTableScan when specifying --query",
	},
	{
		name: "excludeCollection without a db",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = ""
			md.ToolOptions.Collection = ""
			md.OutputOptions.ExcludedCollections = []string{"baz"}
		},
		wantErr: "--db is required when --excludeCollection is specified",
	},
	{
		name: "excludeCollection together with a collection",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = "foo"
			md.ToolOptions.Collection = "baz"
			md.OutputOptions.ExcludedCollections = []string{"baz"}
		},
		wantErr: "--collection is not allowed when --excludeCollection is specified",
	},
	{
		name: "excludeCollectionsWithPrefix without a db",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = ""
			md.ToolOptions.Collection = ""
			md.OutputOptions.ExcludedCollectionPrefixes = []string{"baz"}
		},
		wantErr: "--db is required when --excludeCollectionsWithPrefix is specified",
	},
	{
		name: "excludeCollectionsWithPrefix together with a collection",
		setUp: func(md *MongoDump) {
			md.ToolOptions.DB = "foo"
			md.ToolOptions.Collection = "baz"
			md.OutputOptions.ExcludedCollectionPrefixes = []string{"baz"}
		},
		wantErr: "--collection is not allowed when --excludeCollectionsWithPrefix is specified",
	},
}

// TestMongoDumpQueryFileMissing covers the one query-flag failure that is not an
// option-validation error: the file is only opened when the query is read.
func TestMongoDumpQueryFileMissing(t *testing.T) {
	inputOptions := &InputOptions{
		QueryFile: filepath.Join(t.TempDir(), "does-not-exist.json"),
	}

	_, err := inputOptions.GetQuery()
	assert.ErrorContains(
		t,
		err,
		"error reading queryFile",
		"a --queryFile that does not exist is reported when the query is read",
	)
}

func TestMongoDumpCollectionOutputPath(t *testing.T) {
	t.Skip("disabled: see TOOLS-2658")

	log.SetWriter(io.Discard)

	newDump := func() *MongoDump {
		md, err := simpleMongoDumpInstance()
		require.NoError(t, err)

		md.OutputOptions.Out = "dump"
		return md
	}

	t.Run("paths less than 255 bytes", func(t *testing.T) {
		md := newDump()

		// 26 bytes < 255 bytes
		// (output path will be under 255 bytes, regardless of file extension)
		colName := "abcdefghijklmnopqrstuvwxyz"

		fileComponents := strings.Split(md.outputPath(testDB, colName), "/")
		assert.Len(t, fileComponents, 3)

		filePath := fileComponents[len(fileComponents)-1]
		assert.Equal(t, colName, filePath)
		assert.NotContains(t, filePath, "%24")
	})

	t.Run("paths equal 255 bytes", func(t *testing.T) {
		md := newDump()

		// 17 bytes * 14 = 238 bytes
		// (output would be exactly 255 bytes with longest possible file extension of .metadata.json.gz)
		colName := strings.Repeat("abcdefghijklmnopq", 14)

		fileComponents := strings.Split(md.outputPath(testDB, colName), "/")
		assert.Len(t, fileComponents, 3)

		filePath := fileComponents[len(fileComponents)-1]
		assert.Equal(t, colName, filePath)
		assert.NotContains(t, filePath, "%24")
	})

	t.Run("path longer than 255 bytes", func(t *testing.T) {
		t.Run("without special characters", func(t *testing.T) {
			md := newDump()

			// 26 bytes * 10 = 260 bytes > 238 bytes
			// (output path is already over the file name limit of 255, regardless of file extension)
			colName := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 10)

			fileComponents := strings.Split(md.outputPath(testDB, colName), "/")
			assert.Len(t, fileComponents, 3)

			filePath := fileComponents[len(fileComponents)-1]
			assert.Equal(t, colName[:208]+"%24", filePath[:211])

			hashDecoded, _ := base64.RawURLEncoding.DecodeString(filePath[211:])
			hash := sha1.Sum([]byte(colName))
			assert.Equal(t, hash[:], hashDecoded)
		})

		t.Run("with special characters", func(t *testing.T) {
			md := newDump()

			// (26 bytes + 3 special bytes) * 8 = 232 bytes < 238 bytes
			// (output path is under the limit, but will go over when we escape the special symbols)
			colName := strings.Repeat("abcdefghijklmnopqrstuvwxyz+/@", 8)

			fileComponents := strings.Split(md.outputPath(testDB, colName), "/")
			assert.Len(t, fileComponents, 3)

			filePath := fileComponents[len(fileComponents)-1]
			assert.Equal(t, url.QueryEscape(colName)[:208]+"%24", filePath[:211])

			hashDecoded, _ := base64.RawURLEncoding.DecodeString(filePath[211:])
			hash := sha1.Sum([]byte(colName))
			assert.Equal(t, hash[:], hashDecoded)
		})
	})
}
