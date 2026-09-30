// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestDeleteRemovesEveryVersionOfAFilename checks that one delete removes every
// GridFS file stored under the given name, and the chunks holding their
// contents. A put does not replace an earlier file of the same name unless asked
// to, so a name can stand for many files.
func TestDeleteRemovesEveryVersionOfAFilename(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client := emptyGridFSClient(t)
	localFile := loremIpsumFiles[0]

	for range 10 {
		runPut(t, StorageOptions{}, localFile)
	}
	require.EqualValues(
		t,
		10,
		countFiles(t, client, testDB, "fs.files", bson.D{{"filename", localFile}}),
		"each put stores another GridFS file under the same name",
	)
	require.Positive(
		t,
		countFiles(t, client, testDB, "fs.chunks", bson.D{}),
		"the puts wrote chunks",
	)

	mf := mongoFilesWithStorageOptions(t, "delete", StorageOptions{})
	mf.FileName = localFile
	_, err := mf.Run(false)
	require.NoError(t, err, "can delete %#q", localFile)

	assert.EqualValues(
		t,
		0,
		countFiles(t, client, testDB, "fs.files", bson.D{}),
		"the delete removes every version of the name from fs.files",
	)
	assert.EqualValues(
		t,
		0,
		countFiles(t, client, testDB, "fs.chunks", bson.D{}),
		"the delete leaves no orphaned chunks behind in fs.chunks",
	)
}
