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
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

var (
	testDB     = "mongofiles_test_db"
	testServer = "localhost"
	testPort   = db.DefaultTestPort

	ssl        = testutil.GetSSLOptions()
	auth       = testutil.GetAuthOptions()
	connection = &options.Connection{
		Host: testServer,
		Port: testPort,
	}
	toolOptions = &options.ToolOptions{
		SSL:        &ssl,
		Connection: connection,
		Auth:       &auth,
		Verbosity:  &options.Verbosity{},
		URI:        &options.URI{},
	}
	testFiles = map[string]primitive.ObjectID{"testfile1": primitive.NewObjectID(), "testfile2": primitive.NewObjectID(), "testfile3": primitive.NewObjectID(), "testfile4": primitive.NewObjectID()}
)

// put in some test data into GridFS.
func setUpGridFSTestData() (map[string]int, error) {
	sessionProvider, err := db.NewSessionProvider(*toolOptions)
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
		stream, err := bucket.OpenUploadStreamWithID(id, item)
		if err != nil {
			return nil, err
		}

		n, err := stream.Write([]byte(strings.Repeat("a", (i+1)*5)))
		if err != nil {
			return nil, err
		}

		bytesExpected[item] = n

		if err = stream.Close(); err != nil {
			return nil, err
		}

		i++
	}

	return bytesExpected, nil
}

// remove test data from GridFS.
func tearDownGridFSTestData() error {
	sessionProvider, err := db.NewSessionProvider(*toolOptions)
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
func simpleMongoFilesInstanceWithID(command, ID string) (*MongoFiles, error) {
	return simpleMongoFilesInstanceWithFilenameAndID(command, "", ID)
}
func simpleMongoFilesInstanceWithFilename(command, fname string) (*MongoFiles, error) {
	return simpleMongoFilesInstanceWithFilenameAndID(command, fname, "")
}
func simpleMongoFilesInstanceCommandOnly(command string) (*MongoFiles, error) {
	return simpleMongoFilesInstanceWithFilenameAndID(command, "", "")
}
func simpleMongoFilesInstanceWithMultipleFileNames(command string, fnames ...string) (*MongoFiles, error) {
	mongofiles, err := simpleMongoFilesInstanceCommandOnly(command)
	if err != nil {
		return nil, err
	}

	mongofiles.FileNameList = fnames
	return mongofiles, nil
}

func simpleMongoFilesInstanceWithFilenameAndID(command, fname, ID string) (*MongoFiles, error) {
	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	if err != nil {
		return nil, err
	}

	mongofiles := MongoFiles{
		ToolOptions:     toolOptions,
		InputOptions:    &InputOptions{},
		StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
		SessionProvider: sessionProvider,
		Command:         command,
		FileName:        fname,
		Id:              ID,
	}

	return &mongofiles, nil
}

// simpleMockMongoFilesInstanceWithFilename gets an instance of MongoFiles with no underlying SessionProvider.
// Use this for tests that don't communicate with the server (e.g. options parsing tests).
func simpleMockMongoFilesInstanceWithFilename(command, fname string) *MongoFiles {
	return &MongoFiles{
		ToolOptions:    toolOptions,
		InputOptions:   &InputOptions{},
		StorageOptions: &StorageOptions{GridFSPrefix: "fs", DB: testDB},
		Command:        command,
		FileName:       fname,
	}
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

	file1ContentsBytes, err := io.ReadAll(file1)
	if err != nil {
		return false, err
	}
	file2ContentsBytes, err := io.ReadAll(file2)
	if err != nil {
		return false, err
	}

	isContentSame := bytes.Compare(file1ContentsBytes, file2ContentsBytes) == 0
	return isContentSame, nil

}

// get an id of an existing file, for _id access.
func idOfFile(filename string) string {
	return fmt.Sprintf(`{"$oid":"%s"}`, testFiles[filename].Hex())
}

// test output needs some cleaning.
func cleanAndTokenizeTestOutput(str string) []string {
	// remove last \r\n in str to avoid unnecessary line on split
	if str != "" {
		str = str[:len(str)-1]
	}

	return strings.Split(strings.Trim(str, "\r\n"), "\n")
}

// return slices of files and bytes in each file represented by each line.
func getFilesAndBytesFromLines(lines []string) map[string]int {
	var fileName string
	var byteCount int

	results := make(map[string]int)

	for _, line := range lines {
		//nolint:errcheck
		fmt.Sscanf(line, "%s\t%d", &fileName, &byteCount)
		results[fileName] = byteCount
	}

	return results
}

func getFilesAndBytesListFromGridFS() (map[string]int, error) {
	mfAfter, err := simpleMongoFilesInstanceCommandOnly("list")
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

// check if file exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// Test that it works whenever valid arguments are passed in and that
// it barfs whenever invalid ones are passed.
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

// Test that the output from mongofiles is actually correct.
func TestMongoFilesCommands(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	Convey("Testing the various commands (get|get_id|put|delete|delete_id|search|list) "+
		"with a MongoDump instance", t, func() {

		bytesExpected, err := setUpGridFSTestData()
		So(err, ShouldBeNil)

		Convey("Testing the 'list' command with a file that isn't in GridFS should", func() {
			mf, err := simpleMongoFilesInstanceWithFilename("list", "gibberish")
			So(err, ShouldBeNil)
			So(mf, ShouldNotBeNil)

			Convey("produce no output", func() {
				output, err := mf.Run(false)
				So(err, ShouldBeNil)
				So(len(output), ShouldEqual, 0)
			})
		})

		Convey("Testing the 'list' command with files that are in GridFS should", func() {
			mf, err := simpleMongoFilesInstanceWithFilename("list", "testf")
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

		Convey("Testing the 'search' command with files that are in GridFS should", func() {
			mf, err := simpleMongoFilesInstanceWithFilename("search", "file")
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
			mf, err := simpleMongoFilesInstanceWithFilename("get", "testfile1")
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
				testFile1Bytes, err := io.ReadAll(testFile)
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
			mf, err := simpleMongoFilesInstanceWithMultipleFileNames("get", localTestFiles...)
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

					bytesGotten, err := io.ReadAll(testFile)
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
			_, err := simpleMongoFilesInstanceWithFilename("get", "testfile1")
			So(err, ShouldBeNil)

			id := idOfFile("testfile1")
			mf, err := simpleMongoFilesInstanceWithID("get_id", id)
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
				testFile1Bytes, err := io.ReadAll(testFile)
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
			mf, err := simpleMongoFilesInstanceCommandOnly(GetRegex)
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

			mf, err := simpleMongoFilesInstanceWithMultipleFileNames("put", localTestFiles...)
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
					mfAfter, err := simpleMongoFilesInstanceWithFilename("get", testFile)
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

		Convey("Testing the 'put_id' command by putting some lorem ipsum file with 287613 bytes with different ids should succeed", func() {
			for _, idToTest := range []string{`test_id`, `{"a":"b"}`, `{"$numberLong":"999999999999999"}`, `{"a":{"b":{"c":{}}}}`} {
				runPutIDTestCase(idToTest, t)
			}
		})

		Convey("Testing the 'delete' command with a file that is in GridFS should", func() {
			mf, err := simpleMongoFilesInstanceWithFilename("delete", "testfile2")
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
			_, err := simpleMongoFilesInstanceWithFilename("get", "testfile2")
			So(err, ShouldBeNil)

			idString := idOfFile("testfile2")
			mf, err := simpleMongoFilesInstanceWithID("delete_id", idString)
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
	if ssl.UseSSL {
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

func runPutIDTestCase(idToTest string, t *testing.T) {
	remoteName := "remoteName"
	mongoFilesInstance, err := simpleMongoFilesInstanceWithFilenameAndID("put_id", remoteName, idToTest)

	var buff bytes.Buffer
	log.SetWriter(&buff)

	So(err, ShouldBeNil)
	So(mongoFilesInstance, ShouldNotBeNil)
	mongoFilesInstance.StorageOptions.LocalFileName = util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt")

	t.Log("Should correctly insert the file into GridFS")
	str, err := mongoFilesInstance.Run(false)
	So(err, ShouldBeNil)
	So(str, ShouldEqual, "")
	So(buff.Len(), ShouldNotEqual, 0)

	t.Log("and its filename should exist when the 'list' command is run")
	bytesGotten, err := getFilesAndBytesListFromGridFS()
	So(err, ShouldBeNil)
	So(bytesGotten, ShouldContainKey, remoteName)

	t.Log("and get_id should have exactly the same content as the original file")

	mfAfter, err := simpleMongoFilesInstanceWithID("get_id", idToTest)
	So(err, ShouldBeNil)
	So(mfAfter, ShouldNotBeNil)

	mfAfter.StorageOptions.LocalFileName = "lorem_ipsum_copy.txt"
	buff.Truncate(0)
	str, err = mfAfter.Run(false)
	So(err, ShouldBeNil)
	So(str, ShouldEqual, "")
	So(buff.Len(), ShouldNotEqual, 0)

	loremIpsumOrig, err := os.Open(util.ToUniversalPath("testdata/lorem_ipsum_287613_bytes.txt"))
	So(err, ShouldBeNil)

	loremIpsumCopy, err := os.Open("lorem_ipsum_copy.txt")
	So(err, ShouldBeNil)

	defer loremIpsumOrig.Close()
	defer loremIpsumCopy.Close()

	isContentSame, err := fileContentsCompare(loremIpsumOrig, loremIpsumCopy, t)
	So(err, ShouldBeNil)
	So(isContentSame, ShouldBeTrue)
}
