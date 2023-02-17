// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongofiles

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

var (
	testDB     = "mongofiles_test_db"
	testServer = "localhost"
	testPort   = db.DefaultTestPort

	testFiles = map[string]primitive.ObjectID{"testfile1": primitive.NewObjectID(), "testfile2": primitive.NewObjectID(), "testfile3": primitive.NewObjectID(), "testfile4": primitive.NewObjectID()}
)

func toolOptions() options.ToolOptions {
	connection := &options.Connection{
		Host: testServer,
		Port: testPort,
	}
	ssl := testutil.GetSSLOptions()
	auth := testutil.GetAuthOptions()
	return options.ToolOptions{
		SSL:        &ssl,
		Connection: connection,
		Auth:       &auth,
		Verbosity:  &options.Verbosity{},
		URI:        &options.URI{},
	}
}

type FsFile struct {
	Length   int64 `bson:"length"`
	Metadata struct {
		ContentType string `bson:"contentType"`
	} `bson:"metadata"`
}

// put in some test data into GridFS
func setUpGridFSTestData() (map[string]int, error) {
	sessionProvider, err := db.NewSessionProvider(toolOptions())
	if err != nil {
		return nil, err
	}
	session, err := sessionProvider.GetSession()
	if err != nil {
		return nil, err
	}

	bytesExpected := map[string]int{}

	testDb := session.Database(testDB)
	bucket, err := gridfs.NewBucket(testDb)
	if err != nil {
		return nil, err
	}

	i := 0
	for item, id := range testFiles {
		stream, err := bucket.OpenUploadStreamWithID(id, string(item))
		if err != nil {
			return nil, err
		}

		n, err := stream.Write([]byte(strings.Repeat("a", (i+1)*5)))
		if err != nil {
			return nil, err
		}

		bytesExpected[item] = int(n)

		if err = stream.Close(); err != nil {
			return nil, err
		}

		i++
	}

	return bytesExpected, nil
}

// remove test data from GridFS
func tearDownGridFSTestData() error {
	sessionProvider, err := db.NewSessionProvider(toolOptions())
	if err != nil {
		return err
	}
	session, err := sessionProvider.GetSession()
	if err != nil {
		return err
	}

	if err = session.Database(testDB).Drop(context.Background()); err != nil {
		return err
	}

	return nil
}

func getMongofilesWithArgs(args ...string) (*MongoFiles, error) {
	opts, err := ParseOptions(args, "", "")
	if err != nil {
		return nil, err
	}

	mf, err := New(opts)
	if err != nil {
		return nil, err
	}

	return mf, nil
}

func fileContentsCompare(file1, file2 *os.File, t *testing.T) (bool, error) {
	file1Stat, err := file1.Stat()
	if err != nil {
		return false, err
	}

	file2Stat, err := file2.Stat()
	if err != nil {
		return false, err
	}

	file1Size := file1Stat.Size()
	file2Size := file2Stat.Size()

	if file1Size != file2Size {
		t.Log("file sizes not the same")
		return false, nil
	}

	file1ContentsBytes, err := ioutil.ReadAll(file1)
	if err != nil {
		return false, err
	}
	file2ContentsBytes, err := ioutil.ReadAll(file2)
	if err != nil {
		return false, err
	}

	isContentSame := bytes.Compare(file1ContentsBytes, file2ContentsBytes) == 0
	return isContentSame, nil

}

// get an id of an existing file, for _id access
func idOfFile(filename string) string {
	return fmt.Sprintf(`{"$oid":"%s"}`, testFiles[filename].Hex())
}

// test output needs some cleaning
func cleanAndTokenizeTestOutput(str string) []string {
	// remove last \r\n in str to avoid unnecessary line on split
	if str != "" {
		str = str[:len(str)-1]
	}

	return strings.Split(strings.Trim(str, "\r\n"), "\n")
}

// return slices of files and bytes in each file represented by each line
func getFilesAndBytesFromLines(lines []string) map[string]int {
	var fileName string
	var byteCount int

	results := make(map[string]int)

	for _, line := range lines {
		fmt.Sscanf(line, "%s\t%d", &fileName, &byteCount)
		results[fileName] = byteCount
	}

	return results
}

func getFilesAndBytesListFromGridFS() (map[string]int, error) {
	mfAfter, err := newMongoFilesBuilder("list").Build()
	if err != nil {
		return nil, err
	}
	str, err := mfAfter.Run(false)
	if err != nil {
		return nil, err
	}

	lines := cleanAndTokenizeTestOutput(str)
	results := getFilesAndBytesFromLines(lines)
	return results, nil
}

// check if file exists
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// Test that it works whenever valid arguments are passed in and that
// it barfs whenever invalid ones are passed
func TestValidArguments(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	Convey("With a MongoFiles instance", t, func() {
		mf := simpleMockMongoFilesInstanceWithFilename("search", "file")
		Convey("It should error out when no arguments fed", func() {
			args := []string{}
			err := mf.ValidateCommand(args)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "no command specified")
		})

		Convey("(list|delete|search|get_id|delete_id) should error out when more than 1 positional argument (except URI) is provided", func() {
			for _, command := range []string{"list", "delete", "search", "get_id", "delete_id"} {
				args := []string{command, "arg1", "arg2"}
				err := mf.ValidateCommand(args)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, "too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)")
			}
		})

		Convey("put_id should error out when more than 3 positional argument provided", func() {
			args := []string{"put_id", "arg1", "arg2", "arg3"}
			err := mf.ValidateCommand(args)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)")
		})

		Convey("put_id should error out when only 1 positional argument provided", func() {
			args := []string{"put_id", "arg1"}
			err := mf.ValidateCommand(args)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, fmt.Sprintf("'%v' argument(s) missing", "put_id"))
		})

		Convey("It should not error out when list command isn't given an argument", func() {
			args := []string{"list"}
			So(mf.ValidateCommand(args), ShouldBeNil)
			So(mf.StorageOptions.LocalFileName, ShouldEqual, "")
		})

		Convey("It should not error out when the get command is given multiple supporting arguments", func() {
			args := []string{"get", "foo", "bar", "baz"}
			So(mf.ValidateCommand(args), ShouldBeNil)
			So(mf.FileNameList, ShouldResemble, []string{"foo", "bar", "baz"})
		})

		Convey("It should not error out when the put command is given multiple supporting arguments", func() {
			args := []string{"put", "foo", "bar", "baz"}
			So(mf.ValidateCommand(args), ShouldBeNil)
			So(mf.FileNameList, ShouldResemble, []string{"foo", "bar", "baz"})
		})

		Convey("It should error out when any of (get|put|delete|search|get_id|delete_id) not given supporting argument", func() {
			for _, command := range []string{"get", "put", "delete", "search", "get_id", "delete_id"} {
				args := []string{command}
				err := mf.ValidateCommand(args)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, fmt.Sprintf("'%v' argument missing", command))
			}
		})

		Convey("It should error out when a nonsensical command is given", func() {
			args := []string{"commandnonexistent"}

			err := mf.ValidateCommand(args)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, fmt.Sprintf("'%v' is not a valid command (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)", args[0]))
		})

	})
}

func TestPut(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	t.Run("with filename", testPutWithFilename)
	t.Run("with --local and filename", testPutWithFilenameAndLocal)
	t.Run("with --prefix and filename", testPutWithPrefixAndFilename)
	t.Run("with --replace and filename", testPutWithReplaceAndFilename)
	t.Run("with file that does not exist", testPutWithFileThatDoesNotExist)
	t.Run("with --local file that does not exist", testPutWithLocalAndFileThatDoesNotExist)
	t.Run("with directory instead of file", testPutWithDirectory)
	t.Run("with --type set", testPutWithType)
	t.Run("with large file", testPutWithLargeFile)

	require.NoError(t, tearDownGridFSTestData())
}

func testPutWithFilename(t *testing.T) {
	path := "testdata/lorem_ipsum_multi_args_0.txt"
	testFile := util.ToUniversalPath(path)

	mf, err := newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)

	str, err := mf.Run(false)
	require.NoError(t, err)
	require.Empty(t, str)

	mf, err = newMongoFilesBuilder("list").WithFileName(testFile).Build()
	require.NoError(t, err)

	str, err = mf.Run(false)
	require.NoError(t, err)
	require.Contains(t, str, "testdata/lorem_ipsum_multi_args_0.txt	3411")

	session := getSessionFromMongoFiles(t, mf)
	fileRes := session.Database(testDB).
		Collection("fs.files").
		FindOne(context.Background(), bson.M{"filename": testFile})
	require.NoError(t, fileRes.Err())

	var file FsFile
	err = fileRes.Decode(&file)
	require.NoError(t, err)
	require.Empty(t, file.Metadata.ContentType, "content type is not set when --type is not passed")
}

func testPutWithFilenameAndLocal(t *testing.T) {
	testFile := util.ToUniversalPath("testdata/lorem_ipsum_multi_args_0.txt")

	mf, err := newMongoFilesBuilder("put").WithFileName("new_name.txt").Build()
	require.NoError(t, err)

	mf.StorageOptions.LocalFileName = testFile

	str, err := mf.Run(false)
	require.NoError(t, err)
	require.Empty(t, str)

	mf, err = newMongoFilesBuilder("list").WithFileName("new_name.txt").Build()
	require.NoError(t, err)

	str, err = mf.Run(false)
	require.NoError(t, err)
	require.Contains(t, str, "new_name.txt	3411")
}

func testPutWithPrefixAndFilename(t *testing.T) {
	testFile := util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt")

	mf, err := newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)
	mf.StorageOptions.GridFSPrefix = "prefix_test"

	str, err := mf.Run(false)
	require.NoError(t, err)
	require.Empty(t, str)

	mf, err = newMongoFilesBuilder("list").Build()
	require.NoError(t, err)
	mf.StorageOptions.GridFSPrefix = "prefix_test"

	str, err = mf.Run(false)
	require.NoError(t, err)
	fmt.Println(str)
	require.Contains(t, str, "testdata/lorem_ipsum_287613_bytes.txt	287613")
}

func testPutWithReplaceAndFilename(t *testing.T) {
	require.NoError(t, tearDownGridFSTestData())

	testFile := util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt")

	mf, err := newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)

	for i := 1; i <= 3; i++ {
		str, err := mf.Run(false)
		require.NoError(t, err)
		require.Empty(t, str)
	}

	session := getSessionFromMongoFiles(t, mf)
	count, err := session.Database(testDB).
		Collection("fs.files").
		CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err)
	require.Equal(t, int64(3), count, "by default files with the same name are not replaced")

	require.NoError(t, tearDownGridFSTestData())

	mf, err = newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)
	mf.StorageOptions.Replace = true

	for i := 1; i <= 3; i++ {
		str, err := mf.Run(false)
		require.NoError(t, err)
		require.Empty(t, str)
	}

	count, err = session.Database(testDB).
		Collection("fs.files").
		CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count, "only one file when using --replace")
}

func testPutWithDirectory(t *testing.T) {
	testFile := util.ToUniversalPath("testdata")

	mf, err := newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)

	_, err = mf.Run(false)
	require.ErrorContains(t, err, "error while storing 'testdata' into GridFS: read testdata: is a directory")
}

func testPutWithType(t *testing.T) {
	require.NoError(t, tearDownGridFSTestData())

	testFile := util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt")
	mf, err := newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)

	mf.StorageOptions.ContentType = "text/html"
	_, err = mf.Run(false)

	session := getSessionFromMongoFiles(t, mf)
	fileRes := session.Database(testDB).
		Collection("fs.files").
		FindOne(context.Background(), bson.M{"filename": testFile})
	require.NoError(t, fileRes.Err())

	var file FsFile
	err = fileRes.Decode(&file)
	require.NoError(t, err)
	require.Equal(t, "text/html", file.Metadata.ContentType)
}

func testPutWithLargeFile(t *testing.T) {
	require.NoError(t, tearDownGridFSTestData())

	td, err := ioutil.TempDir("", "mongofiles_")
	require.NoError(t, err)
	defer func() {
		removeErr := os.RemoveAll(td)
		require.NoError(t, removeErr)
	}()

	putFile := filepath.Join(td, "put-file.txt")
	f, err := os.Create(putFile)
	require.NoError(t, err)

	// This creates a 40 megabyte file with some arbitrary content.
	for i := 1; i <= 40; i++ {
		_, err = f.WriteString(strings.Repeat(fmt.Sprintf("%d", i), 1024*1024))
		require.NoError(t, err)
	}

	err = f.Close()
	require.NoError(t, err)

	mf, err := newMongoFilesBuilder("put").WithFileName(putFile).Build()
	require.NoError(t, err)

	_, err = mf.Run(false)
	require.NoError(t, err)

	getFile := filepath.Join(td, "get-file.txt")
	mf, err = newMongoFilesBuilder("get").WithFileName(putFile).Build()
	require.NoError(t, err)

	mf.StorageOptions.LocalFileName = getFile
	_, err = mf.Run(false)
	require.NoError(t, err)

	f, err = os.Open(putFile)
	require.NoError(t, err)

	h := md5.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)

	putSum := h.Sum(nil)

	f, err = os.Open(getFile)
	require.NoError(t, err)

	h = md5.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)

	getSum := h.Sum(nil)

	// We sprintf this to hex to make failures readable.
	require.Equal(t, fmt.Sprintf("%x", putSum), fmt.Sprintf("%x", getSum))

	session := getSessionFromMongoFiles(t, mf)
	fileRes := session.Database(testDB).
		Collection("fs.files").
		FindOne(context.Background(), bson.M{"filename": putFile})
	require.NoError(t, fileRes.Err())

	var file FsFile
	err = fileRes.Decode(&file)
	require.NoError(t, err)

	require.GreaterOrEqual(t, file.Length, int64(40*1024*1024))

	count, err := session.Database(testDB).
		Collection("fs.chunks").
		CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err)

	// Each chunk is a maximum of 255KB.
	require.Equal(t, math.Ceil(float64(file.Length)/(1024*255)), float64(count))
}

func testPutWithFileThatDoesNotExist(t *testing.T) {
	testFile := util.ToUniversalPath("does-not-exist.txt")

	mf, err := newMongoFilesBuilder("put").WithFileName(testFile).Build()
	require.NoError(t, err)

	_, err = mf.Run(false)
	require.ErrorContains(t, err, "error while opening local gridFile 'does-not-exist.txt'")
}

func testPutWithLocalAndFileThatDoesNotExist(t *testing.T) {
	testFile := util.ToUniversalPath("does-not-exist.txt")

	mf, err := newMongoFilesBuilder("put").WithFileName("something.txt").Build()
	require.NoError(t, err)
	mf.StorageOptions.LocalFileName = testFile

	_, err = mf.Run(false)
	require.ErrorContains(t, err, "error while opening local gridFile 'does-not-exist.txt'")
}

func TestPutID(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	require.NoError(t, tearDownGridFSTestData())

	t.Run("with filename", func(t *testing.T) {
		testFile := util.ToUniversalPath("testdata/lorem_ipsum_multi_args_0.txt")

		mf, err := newMongoFilesBuilder("put_id").WithID("42").WithFileName(testFile).Build()
		require.NoError(t, err)

		str, err := mf.Run(false)
		require.NoError(t, err)
		require.Empty(t, str)

		str, err = mf.Run(false)
		require.ErrorContains(t, err, "duplicate key error", "put_id twice with the same id returns an error the second time")
	})

	t.Run("with many different IDs", func(t *testing.T) {
		ids := []string{
			`test_id`,
			`{"a":"b"}`,
			`{"$numberLong":"999999999999999"}`,
			`{"a":{"b":{"c":{}}}}`,
		}
		for _, idToTest := range ids {
			runPutIDTestCase(idToTest, t)
		}
	})
}

func runPutIDTestCase(idToTest string, t *testing.T) {
	remoteName := "remoteName"
	mf, err := newMongoFilesBuilder("put_id").WithID(idToTest).WithFileName(remoteName).Build()

	require.NoError(t, err)
	mf.StorageOptions.LocalFileName = util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt")

	t.Run("insert the file", func(t *testing.T) {
		str, err := mf.Run(false)
		require.NoError(t, err)
		require.Empty(t, str)
	})

	t.Run("list sees the file", func(t *testing.T) {
		bytesGotten, err := getFilesAndBytesListFromGridFS()
		require.NoError(t, err)
		require.Contains(t, bytesGotten, remoteName)
	})

	t.Run("get_id finds same content as original", func(t *testing.T) {
		mfAfter, err := newMongoFilesBuilder("get_id").WithID(idToTest).Build()
		require.NoError(t, err)

		mfAfter.StorageOptions.LocalFileName = "lorem_ipsum_copy.txt"

		str, err := mfAfter.Run(false)
		require.NoError(t, err)
		require.Empty(t, str)

		loremIpsumOrig, err := os.Open(util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt"))
		require.NoError(t, err)

		loremIpsumCopy, err := os.Open("lorem_ipsum_copy.txt")
		require.NoError(t, err)

		defer loremIpsumOrig.Close()
		defer loremIpsumCopy.Close()

		isContentSame, err := fileContentsCompare(loremIpsumOrig, loremIpsumCopy, t)
		require.NoError(t, err)
		require.True(t, isContentSame)
	})
}

// Test that the output from mongofiles is actually correct
func TestMongoFilesCommands(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	Convey("Testing the various commands (get|get_id|put|delete|delete_id|search|list) "+
		"with a MongoFiles instance", t, func() {

		bytesExpected, err := setUpGridFSTestData()
		So(err, ShouldBeNil)

		Convey("Testing the 'list' command with a file that isn't in GridFS should", func() {
			mf, err := newMongoFilesBuilder("list").WithFileName("gibberish").Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			Convey("produce no output", func() {
				output, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(len(output), ShouldEqual, 0)
			})
		})

		Convey("Testing the 'list' command with files that are in GridFS should", func() {
			mf, err := newMongoFilesBuilder("list").WithFileName("testf").Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			Convey("produce some output", func() {
				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(len(str), ShouldNotEqual, 0)

				lines := cleanAndTokenizeTestOutput(str)
				So(len(lines), ShouldEqual, len(testFiles))

				bytesGotten := getFilesAndBytesFromLines(lines)
				So(bytesGotten, ShouldResemble, bytesExpected)
			})
		})

		Convey("Testing the 'get' command with a file that is in GridFS should", func() {
			mf, err := newMongoFilesBuilder("get").WithFileName("testfile1").Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			var buff bytes.Buffer
			log.SetWriter(&buff)

			Convey("store the file contents in a file with different name if '--local' flag used", func() {
				buff.Truncate(0)
				mf.StorageOptions.LocalFileName = "testfile1copy"
				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(str, ShouldEqual, "")
				So(buff.Len(), ShouldNotEqual, 0)

				testFile, err := os.Open("testfile1copy")
				So(err, ShouldBeNil)
				defer testFile.Close()

				// pretty small file; so read all
				testFile1Bytes, err := ioutil.ReadAll(testFile)
				So(err, ShouldBeNil)
				So(len(testFile1Bytes), ShouldEqual, bytesExpected["testfile1"])
			})

			// cleanup file we just copied to the local FS
			Reset(func() {

				// remove 'testfile1' or 'testfile1copy'
				if fileExists("testfile1") {
					err = os.Remove("testfile1")
				}
				So(err, ShouldBeNil)

				if fileExists("testfile1copy") {
					err = os.Remove("testfile1copy")
				}
				So(err, ShouldBeNil)

			})
		})

		Convey("Testing the 'get' command with multiple files that are in GridFS should", func() {
			localTestFiles := []string{"testfile1", "testfile2", "testfile3"}
			mf, err := newMongoFilesBuilder("get").WithFileNames(localTestFiles).Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			var buff bytes.Buffer
			log.SetWriter(&buff)

			str, err := mf.Run(false)
			So(err, ShouldBeNil)
			So(str, ShouldBeEmpty)

			Convey("log an event specifying the completion of each file", func() {
				logOutput := buff.String()

				for _, testFile := range localTestFiles {
					logEvent := fmt.Sprintf("finished writing to %v", testFile)
					So(logOutput, ShouldContainSubstring, logEvent)
				}
			})

			Convey("copy the files to the local filesystem", func() {
				for _, testFileName := range localTestFiles {
					testFile, err := os.Open(testFileName)
					So(err, ShouldBeNil)
					defer testFile.Close()

					bytesGotten, err := ioutil.ReadAll(testFile)
					So(err, ShouldBeNil)
					So(len(bytesGotten), ShouldEqual, bytesExpected[testFileName])
				}

				// Make sure that only the files that we queried
				// for are included in the local FS
				unincludedTestFile := "testfile4"
				_, err := os.Open(unincludedTestFile)
				So(err, ShouldNotBeNil)
			})

			// Remove test files from local FS so that there
			// no naming collisions
			Reset(func() {
				for _, testFile := range localTestFiles {
					if fileExists(testFile) {
						err = os.Remove(testFile)
						So(err, ShouldBeNil)
					}
				}
			})
		})

		Convey("Testing the 'get_id' command with a file that is in GridFS should", func() {
			mf, _ := newMongoFilesBuilder("get").WithFileName("testfile1").Build()
			id := idOfFile("testfile1")

			So(err, ShouldBeNil)
			idString := id

			mf, err = newMongoFilesBuilder("get_id").WithID(idString).Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			var buff bytes.Buffer
			log.SetWriter(&buff)

			Convey("copy the file to the local filesystem", func() {
				buff.Truncate(0)
				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(str, ShouldEqual, "")
				So(buff.Len(), ShouldNotEqual, 0)

				testFile, err := os.Open("testfile1")
				So(err, ShouldBeNil)
				defer testFile.Close()

				// pretty small file; so read all
				testFile1Bytes, err := ioutil.ReadAll(testFile)
				So(err, ShouldBeNil)
				So(len(testFile1Bytes), ShouldEqual, bytesExpected["testfile1"])
			})

			Reset(func() {
				// remove 'testfile1' or 'testfile1copy'
				if fileExists("testfile1") {
					err = os.Remove("testfile1")
				}
				So(err, ShouldBeNil)
				if fileExists("testfile1copy") {
					err = os.Remove("testfile1copy")
				}
				So(err, ShouldBeNil)
			})
		})

		Convey("Testing the 'get_regex' command should", func() {
			mf, err := newMongoFilesBuilder(GetRegex).Build()
			So(err, ShouldBeNil)

			Convey("return expected test files, but no others, when called without any server options", func() {
				mf.FileNameRegex = "testfile[1-3]"

				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(str, ShouldBeEmpty)

				// Regex should get all testfiles but testfile4
				expectedTestFiles := map[string]struct{}{
					"testfile1": {},
					"testfile2": {},
					"testfile3": {},
				}

				for testFile := range testFiles {
					_, err := os.Stat(testFile)
					if _, ok := expectedTestFiles[testFile]; ok {
						So(err, ShouldBeNil)
					} else {
						So(err, ShouldNotBeNil)
					}
				}
			})

			Convey("return expected test files, but no others, when called with server options", func() {
				// Check with case-insensitivity
				mf.FileNameRegex = "tEsTfIlE[1-2]"
				mf.StorageOptions.RegexOptions = "i"

				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(str, ShouldBeEmpty)

				expectedTestFiles := map[string]struct{}{
					"testfile1": {},
					"testfile2": {},
				}

				for testFile := range testFiles {
					_, err := os.Stat(testFile)
					if _, ok := expectedTestFiles[testFile]; ok {
						So(err, ShouldBeNil)
					} else {
						So(err, ShouldNotBeNil)
					}
				}
			})

			Reset(func() {
				// Remove any testfiles written to local filesystem
				for testFile := range testFiles {
					if _, err := os.Stat(testFile); err == nil {
						err = os.Remove(testFile)
						So(err, ShouldBeNil)
					}
				}
			})
		})

		Convey("Testing the 'put' command with multiple lorem ipsum files bytes should", func() {
			localTestFiles := []string{
				util.ToUniversalPath("testdata/lorem_ipsum_multi_args_0.txt"),
				util.ToUniversalPath("testdata/lorem_ipsum_multi_args_1.txt"),
				util.ToUniversalPath("testdata/lorem_ipsum_multi_args_2.txt"),
			}

			mf, err := newMongoFilesBuilder("put").WithFileNames(localTestFiles).Build()
			So(err, ShouldBeNil)

			var buff bytes.Buffer
			log.SetWriter(&buff)

			str, err := mf.Run(false)
			So(err, ShouldBeNil)
			So(str, ShouldBeEmpty)

			Convey("log an event specifying the completion of each file", func() {
				const (
					logAdding = "adding gridFile: %v"
					logAdded  = "added gridFile: %v"
				)

				logOutput := buff.String()

				for _, testFile := range localTestFiles {
					So(logOutput, ShouldContainSubstring, fmt.Sprintf(logAdding, testFile))
					So(logOutput, ShouldContainSubstring, fmt.Sprintf(logAdded, testFile))

				}
			})

			Convey("and files should exist in GridFS", func() {
				bytesGotten, err := getFilesAndBytesListFromGridFS()
				So(err, ShouldBeNil)

				// Check that the only files included are the local test
				// files, i.e. in localTestFiles, and the global test
				// files, i.e. in testFiles
				So(len(bytesGotten), ShouldEqual, len(localTestFiles)+len(testFiles))

				for _, testFile := range localTestFiles {
					So(bytesGotten, ShouldContainKey, testFile)
				}
			})

			Convey("and each file should have exactly the same content as the original file", func() {
				const localFileName = "lorem_ipsum_copy.txt"
				buff.Truncate(0)
				for _, testFile := range localTestFiles {
					mfAfter, err := newMongoFilesBuilder("get").WithFileName(testFile).Build()
					So(err, ShouldBeNil)
					So(mf, ShouldNotBeNil)

					mfAfter.StorageOptions.LocalFileName = localFileName
					str, err = mfAfter.Run(false)
					So(err, ShouldBeNil)
					So(str, ShouldBeEmpty)

					testName := fmt.Sprintf("compare contents of %v and original lorem ipsum file", testFile)
					Convey(testName, func() {
						loremIpsumOrig, err := os.Open(testFile)
						So(err, ShouldBeNil)
						defer loremIpsumOrig.Close()

						loremIpsumCopy, err := os.Open(localFileName)
						So(err, ShouldBeNil)
						defer loremIpsumCopy.Close()

						isContentSame, err := fileContentsCompare(loremIpsumOrig, loremIpsumCopy, t)
						So(err, ShouldBeNil)
						So(isContentSame, ShouldBeTrue)
					})
				}

				Reset(func() {
					err = os.Remove(localFileName)
					So(err, ShouldBeNil)
				})

			})
		})

		Convey("Testing the 'delete' command with a file that is in GridFS should", func() {
			mf, err := newMongoFilesBuilder("delete").WithFileName("testfile2").Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			var buff bytes.Buffer
			log.SetWriter(&buff)

			Convey("delete the file from GridFS", func() {
				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(str, ShouldEqual, "")
				So(buff.Len(), ShouldNotEqual, 0)

				Convey("check that the file has been deleted from GridFS", func() {
					bytesGotten, err := getFilesAndBytesListFromGridFS()
					So(err, ShouldEqual, nil)
					So(len(bytesGotten), ShouldEqual, len(testFiles)-1)

					So(bytesGotten, ShouldNotContainKey, "testfile2")
				})
			})
		})

		Convey("Testing the 'delete_id' command with a file that is in GridFS should", func() {
			// hack to grab an _id
			mf, _ := newMongoFilesBuilder("get").WithFileName("testfile2").Build()
			idString := idOfFile("testfile2")

			mf, err := newMongoFilesBuilder("delete_id").WithID(idString).Build()
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			var buff bytes.Buffer
			log.SetWriter(&buff)

			Convey("delete the file from GridFS", func() {
				str, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(str, ShouldEqual, "")
				So(buff.Len(), ShouldNotEqual, 0)

				Convey("check that the file has been deleted from GridFS", func() {
					bytesGotten, err := getFilesAndBytesListFromGridFS()
					So(err, ShouldEqual, nil)
					So(len(bytesGotten), ShouldEqual, len(testFiles)-1)

					So(bytesGotten, ShouldNotContainKey, "testfile2")
				})
			})
		})

		Reset(func() {
			So(tearDownGridFSTestData(), ShouldBeNil)
			err = os.Remove("lorem_ipsum_copy.txt")
		})
	})

}

// Test that when no write concern is specified, a majority write concern is set.
func TestDefaultWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	opts := toolOptions()
	if opts.SSL.UseSSL {
		t.Skip("Skipping non-SSL test with SSL configuration")
	}

	Convey("with a URI that doesn't specify write concern", t, func() {
		mf, err := getMongofilesWithArgs("get", "filename", "--uri", "mongodb://localhost:33333")
		So(err, ShouldBeNil)
		So(mf.SessionProvider.DB("test").WriteConcern(), ShouldResemble, writeconcern.New(writeconcern.WMajority()))
	})

	Convey("with no URI and no write concern option", t, func() {
		mf, err := getMongofilesWithArgs("get", "filename", "--port", "33333")
		So(err, ShouldBeNil)
		So(mf.SessionProvider.DB("test").WriteConcern(), ShouldResemble, writeconcern.New(writeconcern.WMajority()))
	})
}

func TestInvalidHostnameAndPort(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_, err := getMongofilesWithArgs("get", "filename", "--host", "not-a-valid-hostname")
	assert.ErrorContains(t, err, "error connecting to host")

	_, err = getMongofilesWithArgs("get", "filename", "--host", "555.555.555.555")
	assert.ErrorContains(t, err, "error connecting to host")

	_, err = getMongofilesWithArgs("get", "filename", "--host", "localhost", "--port", "12345")
	assert.ErrorContains(t, err, "error connecting to host")
}

func TestSearch(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	require.NoError(t, tearDownGridFSTestData())

	filesExpected, err := setUpGridFSTestData()
	require.NoError(t, err)

	t.Run("search string one file", func(*testing.T) {
		for file, size := range filesExpected {
			t.Run(fmt.Sprintf(`searching for "%s"`, file), func(t *testing.T) {
				mf, err := newMongoFilesBuilder("search").WithFileName(file).Build()
				require.NoError(t, err)

				str, err := mf.Run(false)
				require.NoError(t, err)
				require.Greater(t, len(str), 0)

				bytesGotten := getFilesAndBytesFromLines(cleanAndTokenizeTestOutput(str))
				expect := map[string]int{file: size}
				require.Equal(t, expect, bytesGotten)
			})
		}
	})

	t.Run("search string matches all files", func(*testing.T) {
		for _, s := range []string{"file", "ile", "test"} {
			t.Run(fmt.Sprintf(`searching for "%s"`, s), func(t *testing.T) {
				mf, err := newMongoFilesBuilder("search").WithFileName(s).Build()
				require.NoError(t, err)

				str, err := mf.Run(false)
				require.NoError(t, err)
				require.Greater(t, len(str), 0)

				bytesGotten := getFilesAndBytesFromLines(cleanAndTokenizeTestOutput(str))
				require.Equal(t, filesExpected, bytesGotten)
			})
		}
	})

	t.Run("non-matching searching strings", func(*testing.T) {
		for _, s := range []string{"random", "120549u12905", "filers"} {
			t.Run(fmt.Sprintf(`searching for "%s"`, s), func(t *testing.T) {
				mf, err := newMongoFilesBuilder("search").WithFileName(s).Build()
				require.NoError(t, err)

				str, err := mf.Run(false)
				require.NoError(t, err)
				require.Empty(t, str)
			})
		}
	})
}

func TestWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	concerns := []*writeconcern.WriteConcern{
		writeconcern.New(writeconcern.WMajority()),
		writeconcern.New(writeconcern.W(1), writeconcern.WTimeout(10000)),
		writeconcern.New(writeconcern.W(2), writeconcern.WTimeout(10000)),
	}
	for _, c := range concerns {
		mf, err := newMongoFilesBuilder("put").
			WithFileName("irrelevant").
			WithWriteConcern(c).
			Build()
		require.NoError(t, err)

		session := getSessionFromMongoFiles(t, mf)
		require.Equal(t, c, session.Database("admin").WriteConcern())
	}
}

func getSessionFromMongoFiles(t *testing.T, mf *MongoFiles) *mongo.Client {
	session, err := mf.SessionProvider.GetSession()
	require.NoError(t, err)

	return session
}

type mongoFilesBuilder struct {
	command      string
	id           string
	fileName     string
	fileNameList []string
	opts         options.ToolOptions
}

func newMongoFilesBuilder(command string) *mongoFilesBuilder {
	return &mongoFilesBuilder{
		command: command,
		opts:    toolOptions(),
	}
}

func (mfb *mongoFilesBuilder) WithID(id string) *mongoFilesBuilder {
	mfb.id = id
	return mfb
}

func (mfb *mongoFilesBuilder) WithFileName(fileName string) *mongoFilesBuilder {
	mfb.fileName = fileName
	return mfb
}

func (mfb *mongoFilesBuilder) WithFileNames(filenames []string) *mongoFilesBuilder {
	mfb.fileNameList = filenames
	return mfb
}

func (mfb *mongoFilesBuilder) WithWriteConcern(writeConcern *writeconcern.WriteConcern) *mongoFilesBuilder {
	mfb.opts.WriteConcern = writeConcern
	return mfb
}

func (mfb *mongoFilesBuilder) Build() (*MongoFiles, error) {
	sessionProvider, err := db.NewSessionProvider(mfb.opts)
	if err != nil {
		return nil, err
	}

	return &MongoFiles{
		ToolOptions:     &mfb.opts,
		InputOptions:    &InputOptions{},
		StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
		SessionProvider: sessionProvider,
		Command:         mfb.command,
		FileName:        mfb.fileName,
		FileNameList:    mfb.fileNameList,
		Id:              mfb.id,
	}, nil

}

// simpleMockMongoFilesInstanceWithFilename gets an instance of MongoFiles with no underlying SessionProvider.
// Use this for tests that don't communicate with the server (e.g. options parsing tests)
func simpleMockMongoFilesInstanceWithFilename(command, fname string) *MongoFiles {
	opts := toolOptions()
	return &MongoFiles{
		ToolOptions:    &opts,
		InputOptions:   &InputOptions{},
		StorageOptions: &StorageOptions{GridFSPrefix: "fs", DB: testDB},
		Command:        command,
		FileName:       fname,
	}
}
