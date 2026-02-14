// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoimport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestGetJSONFilesFromDirUnit(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	require.NoError(os.WriteFile(filepath.Join(dir, "a.json"), []byte("{}"), 0o644))
	require.NoError(os.WriteFile(filepath.Join(dir, "b.JSON"), []byte("{}"), 0o644))
	require.NoError(os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not json"), 0o644))

	imp := NewMockMongoImport()
	files, err := imp.getJSONFilesFromDir(dir)
	require.NoError(err)

	found := map[string]bool{}
	for _, f := range files {
		found[filepath.Base(f)] = true
	}
	require.True(found["a.json"])
	require.True(found["b.JSON"])
	require.False(found["c.txt"])
}

func TestGetJSONFilesFromDirNoJSONUnit(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	require.NoError(os.WriteFile(filepath.Join(dir, "only.txt"), []byte("no json here"), 0o644))

	imp := NewMockMongoImport()
	_, err := imp.getJSONFilesFromDir(dir)
	require.Error(err)
	require.Contains(err.Error(), "no JSON files found in directory")
}

func TestImportFromDirectoryIntegration(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	// Create two files with three documents total
	require.NoError(os.WriteFile(filepath.Join(dir, "f1.json"), []byte("{"+"\"_id\":1,\"a\":1}\n{"+"\"_id\":2,\"a\":2}\n"), 0o644))
	require.NoError(os.WriteFile(filepath.Join(dir, "f2.json"), []byte("{"+"\"_id\":3,\"a\":3}\n"), 0o644))

	imp, err := getImportWithArgs("--dir", dir)
	require.NoError(err)

	imp.IngestOptions.Mode = modeInsert
	imp.IngestOptions.WriteConcern = "majority"

	numProcessed, numFailed, err := imp.ImportDocuments()
	require.NoError(err)
	require.Equal(uint64(3), numProcessed)
	require.Equal(uint64(0), numFailed)

	expectedDocuments := []bson.M{
		{"_id": int32(1), "a": int32(1)},
		{"_id": int32(2), "a": int32(2)},
		{"_id": int32(3), "a": int32(3)},
	}
	require.NoError(checkOnlyHasDocuments(imp.SessionProvider, expectedDocuments))
}
