// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var loremIpsumFiles = []string{
	filepath.FromSlash("testdata/lorem_ipsum_multi_args_0.txt"),
	filepath.FromSlash("testdata/lorem_ipsum_multi_args_1.txt"),
	filepath.FromSlash("testdata/lorem_ipsum_multi_args_2.txt"),
}

func TestPutReplace(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client := emptyGridFSClient(t)
	repeated := loremIpsumFiles[0]

	for range 3 {
		runPut(t, StorageOptions{}, repeated)
	}
	assert.EqualValues(
		t,
		3,
		countFiles(t, client, testDB, "fs.files", bson.D{{"filename", repeated}}),
		"putting the same file three times leaves three GridFS files",
	)

	runPut(t, StorageOptions{Replace: true}, repeated)
	assert.EqualValues(
		t,
		1,
		countFiles(t, client, testDB, "fs.files", bson.D{{"filename", repeated}}),
		"put --replace leaves exactly one GridFS file for that name",
	)

	for _, name := range loremIpsumFiles[1:] {
		runPut(t, StorageOptions{}, name)
	}
	runPut(t, StorageOptions{Replace: true}, repeated)

	for _, name := range loremIpsumFiles {
		assert.EqualValues(
			t,
			1,
			countFiles(t, client, testDB, "fs.files", bson.D{{"filename", name}}),
			"put --replace leaves one copy of %#q and does not touch the other files",
			name,
		)
	}
}

func TestPutIDRejectsDuplicateID(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	client := emptyGridFSClient(t)
	const duplicateID = "1"

	mf := mongoFilesForPut(t, StorageOptions{}, loremIpsumFiles[0])
	mf.Command = "put_id"
	mf.Id = duplicateID
	_, err := mf.Run(false)
	require.NoError(t, err, "the first put_id with this _id succeeds")

	ownChunks := bson.D{{"files_id", duplicateID}}
	chunksBefore := countFiles(t, client, testDB, "fs.chunks", ownChunks)
	require.Positive(t, chunksBefore, "the first put_id wrote chunks")

	mf = mongoFilesForPut(t, StorageOptions{}, loremIpsumFiles[0])
	mf.Command = "put_id"
	mf.Id = duplicateID
	_, err = mf.Run(false)
	require.ErrorContains(
		t,
		err,
		"duplicate key",
		"a second put_id with the same _id fails",
	)

	assert.Equal(
		t,
		chunksBefore,
		countFiles(t, client, testDB, "fs.chunks", ownChunks),
		"the failed put_id leaves the existing chunks alone",
	)
}

func TestGridFSPrefix(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const prefix = "custom"

	client := emptyGridFSClient(t)
	localFile := loremIpsumFiles[0]

	runPut(t, StorageOptions{}, localFile)
	assert.EqualValues(
		t,
		1,
		countFiles(t, client, testDB, "fs.files", bson.D{}),
		"a put without --prefix writes to fs.files",
	)
	assert.EqualValues(
		t,
		0,
		countFiles(t, client, testDB, prefix+".files", bson.D{}),
		"a put without --prefix does not write to the custom prefix",
	)

	runPut(t, StorageOptions{GridFSPrefix: prefix}, localFile)
	assert.EqualValues(
		t,
		1,
		countFiles(t, client, testDB, prefix+".files", bson.D{}),
		"a put with --prefix writes to the prefixed files collection",
	)
	assert.EqualValues(
		t,
		1,
		countFiles(t, client, testDB, prefix+".chunks", bson.D{}),
		"a put with --prefix writes to the prefixed chunks collection",
	)
	assert.EqualValues(
		t,
		1,
		countFiles(t, client, testDB, "fs.files", bson.D{}),
		"a put with --prefix does not add to fs.files",
	)

	localCopy := filepath.Join(t.TempDir(), "prefix-copy.txt")
	mf := mongoFilesWithStorageOptions(
		t,
		"get",
		StorageOptions{GridFSPrefix: prefix, LocalFileName: localCopy},
	)
	mf.FileName = localFile
	_, err := mf.Run(false)
	require.NoError(t, err, "a get with --prefix finds the file")

	assertFilesIdentical(t, localFile, localCopy)
}

func TestPutContentType(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const contentType = "text/html"

	client := emptyGridFSClient(t)
	typedFile := loremIpsumFiles[0]
	untypedFile := loremIpsumFiles[1]

	runPut(t, StorageOptions{ContentType: contentType}, typedFile)
	runPut(t, StorageOptions{}, untypedFile)

	assert.Equal(
		t,
		contentType,
		gridFSFile(t, client, typedFile).Metadata.ContentType,
		"put --type stores the content type in the file's metadata",
	)
	assert.Empty(
		t,
		gridFSFile(t, client, untypedFile).Metadata.ContentType,
		"a put without --type stores no content type",
	)

	localCopy := filepath.Join(t.TempDir(), "typed-copy.txt")
	mf := mongoFilesWithStorageOptions(t, "get", StorageOptions{LocalFileName: localCopy})
	mf.FileName = typedFile
	_, err := mf.Run(false)
	require.NoError(t, err, "a file put with --type can be fetched")

	assertFilesIdentical(t, typedFile, localCopy)
}

func TestPutIntoOtherDB(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const otherDB = "mongofiles_test_other_db"

	client := emptyGridFSClient(t)
	dropDatabase(t, client, otherDB)
	t.Cleanup(func() { dropDatabase(t, client, otherDB) })

	for range 2 {
		runPut(t, StorageOptions{DB: otherDB}, loremIpsumFiles[0])
	}

	assert.EqualValues(
		t,
		2,
		countFiles(t, client, otherDB, "fs.files", bson.D{}),
		"both puts landed in the database named by --db",
	)
	assert.EqualValues(
		t,
		0,
		countFiles(t, client, testDB, "fs.files", bson.D{}),
		"the default database is untouched by a put with --db",
	)
}

// TestPutLocalFileName checks that with --local the local file supplies the
// bytes while the positional argument names the GridFS file.
func TestPutLocalFileName(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const remoteName = "remote-lorem-ipsum"

	client := emptyGridFSClient(t)
	localFile := loremIpsumFiles[0]

	mf := mongoFilesWithStorageOptions(t, "put", StorageOptions{LocalFileName: localFile})
	mf.FileName = remoteName
	_, err := mf.Run(false)
	require.NoError(t, err, "a put with --local succeeds")

	assert.EqualValues(
		t,
		1,
		countFiles(t, client, testDB, "fs.files", bson.D{{"filename", remoteName}}),
		"the file is stored under the name given as the positional argument",
	)

	missingLocal := filepath.Join(t.TempDir(), "does-not-exist.txt")
	mf = mongoFilesWithStorageOptions(t, "put", StorageOptions{LocalFileName: missingLocal})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.ErrorContains(
		t,
		err,
		"error while opening local gridFile",
		"a put whose --local file does not exist fails",
	)

	localCopy := filepath.Join(t.TempDir(), "local-copy.txt")
	mf = mongoFilesWithStorageOptions(t, "get", StorageOptions{LocalFileName: localCopy})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.NoError(t, err, "a get with --local succeeds")

	assertFilesIdentical(t, localFile, localCopy)
}

// TestEmptyLocalFileName covers the fallback in getLocalFileName: an empty
// --local means the GridFS name is used as the local path too.
func TestEmptyLocalFileName(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const remoteName = "empty-local.txt"

	emptyGridFSClient(t)

	localFile, err := filepath.Abs(loremIpsumFiles[0])
	require.NoError(t, err, "can find the absolute path of the local file")

	mf := mongoFilesWithStorageOptions(t, "put", StorageOptions{LocalFileName: localFile})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.NoError(t, err, "can put the file under a name of its own")

	mf = mongoFilesWithStorageOptions(t, "put", StorageOptions{})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.ErrorContains(
		t,
		err,
		"error while opening local gridFile",
		"a put with an empty --local fails when no local file has the GridFS name",
	)

	// The get below writes to a relative path, and mongofiles refuses to write
	// outside the working directory.
	t.Chdir(t.TempDir())

	mf = mongoFilesWithStorageOptions(t, "get", StorageOptions{})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.NoError(t, err, "a get with an empty --local succeeds")

	assertFilesIdentical(t, localFile, remoteName)
}

// TestPutLargeFile checks that a file far larger than one GridFS chunk is split
// into the expected number of chunks and comes back byte for byte.
func TestPutLargeFile(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		largeFileSize = 40 * 1024 * 1024
		chunkSize     = 255 * 1024
		remoteName    = "large-file"
	)

	client := emptyGridFSClient(t)
	tmpDir := t.TempDir()

	largeFile := filepath.Join(tmpDir, "large.bin")
	require.NoError(
		t,
		os.WriteFile(largeFile, largeFileContents(largeFileSize), 0o600),
		"can write the large local file",
	)

	mf := mongoFilesWithStorageOptions(t, "put", StorageOptions{})
	mf.FileName = tmpDir
	_, err := mf.Run(false)
	// The put path formats the io.Copy error with %v, so the
	// tail of the message is whatever the OS says about reading an open directory:
	// "is a directory" on Linux, "Incorrect function." on Windows. Only the part
	// mongofiles itself writes is portable.
	require.ErrorContains(
		t,
		err,
		fmt.Sprintf("error while storing %#q into GridFS", tmpDir),
		"putting a directory fails",
	)

	mf = mongoFilesWithStorageOptions(t, "put", StorageOptions{LocalFileName: largeFile})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.NoError(t, err, "can put a 40MB file")

	stored := gridFSFile(t, client, remoteName)
	require.EqualValues(
		t,
		largeFileSize,
		stored.Length,
		"the whole local file was stored",
	)

	expectedChunks := (stored.Length + chunkSize - 1) / chunkSize
	assert.EqualValues(
		t,
		expectedChunks,
		countFiles(t, client, testDB, "fs.chunks", bson.D{{"files_id", stored.ID}}),
		"the large file is stored as ceil(length/chunkSize) chunks",
	)

	localCopy := filepath.Join(tmpDir, "large-copy.bin")
	mf = mongoFilesWithStorageOptions(t, "get", StorageOptions{LocalFileName: localCopy})
	mf.FileName = remoteName
	_, err = mf.Run(false)
	require.NoError(t, err, "the large file can be fetched again")

	assertFilesIdentical(t, largeFile, localCopy)
}

func largeFileContents(size int) []byte {
	block := []byte("mongoDB")

	contents := make([]byte, 0, size+len(block))
	for len(contents) < size {
		contents = append(contents, block...)
	}

	return contents[:size]
}

func runPut(t *testing.T, storage StorageOptions, localFile string) {
	t.Helper()

	mf := mongoFilesForPut(t, storage, localFile)
	_, err := mf.Run(false)
	require.NoError(t, err, "can put %#q", localFile)
}

func mongoFilesForPut(t *testing.T, storage StorageOptions, localFile string) *MongoFiles {
	t.Helper()

	mf := mongoFilesWithStorageOptions(t, "put", storage)
	mf.FileName = localFile

	return mf
}

// mongoFilesWithStorageOptions builds a MongoFiles for command, filling in the
// prefix and database defaults that the command line would otherwise supply.
func mongoFilesWithStorageOptions(
	t *testing.T,
	command string,
	storage StorageOptions,
) *MongoFiles {
	t.Helper()

	if storage.GridFSPrefix == "" {
		storage.GridFSPrefix = "fs"
	}
	if storage.DB == "" {
		storage.DB = testDB
	}

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	require.NoError(t, err, "can create a session provider")

	return &MongoFiles{
		ToolOptions:     toolOptions,
		InputOptions:    &InputOptions{},
		StorageOptions:  &storage,
		SessionProvider: sessionProvider,
		Command:         command,
	}
}

// emptyGridFSClient returns a client whose test database has been dropped, so
// each test starts with no GridFS files, and drops it again when the test ends.
func emptyGridFSClient(t *testing.T) *mongo.Client {
	t.Helper()

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	require.NoError(t, err, "can create a session provider")

	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "can get a session")

	dropDatabase(t, client, testDB)
	t.Cleanup(func() { dropDatabase(t, client, testDB) })

	return client
}

func dropDatabase(t *testing.T, client *mongo.Client, dbName string) {
	t.Helper()

	// Not t.Context(): this also runs from t.Cleanup, where that context has
	// already been canceled.
	require.NoError(
		t,
		client.Database(dbName).Drop(context.Background()),
		"can drop %#q",
		dbName,
	)
}

func countFiles(
	t *testing.T,
	client *mongo.Client,
	dbName, collName string,
	filter bson.D,
) int64 {
	t.Helper()

	count, err := client.Database(dbName).Collection(collName).CountDocuments(t.Context(), filter)
	require.NoError(t, err, "can count the documents in %s.%s", dbName, collName)

	return count
}

func gridFSFile(t *testing.T, client *mongo.Client, filename string) gfsFile {
	t.Helper()

	var file gfsFile
	err := client.Database(testDB).
		Collection("fs.files").
		FindOne(t.Context(), bson.D{{"filename", filename}}).
		Decode(&file)
	require.NoError(t, err, "can find the GridFS file %#q", filename)

	return file
}

func assertFilesIdentical(t *testing.T, wantPath, gotPath string) {
	t.Helper()

	want, err := os.ReadFile(wantPath)
	require.NoError(t, err, "can read %#q", wantPath)

	got, err := os.ReadFile(gotPath)
	require.NoError(t, err, "can read %#q", gotPath)

	assert.True(
		t,
		bytes.Equal(want, got),
		"the contents of %#q and %#q are identical",
		wantPath,
		gotPath,
	)
}
