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
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
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
		SSL:          &ssl,
		Connection:   connection,
		Auth:         &auth,
		Verbosity:    &options.Verbosity{},
		URI:          &options.URI{},
		WriteConcern: wcwrapper.Majority(),
	}
	testFiles = map[string]bson.ObjectID{
		"testfile1": bson.NewObjectID(),
		"testfile2": bson.NewObjectID(),
		"testfile3": bson.NewObjectID(),
		"testfile4": bson.NewObjectID(),
	}
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
	bucket := testDb.GridFSBucket()

	i := 0
	for item, id := range testFiles {
		stream, err := bucket.OpenUploadStreamWithID(context.TODO(), id, item)
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

// remove test data from GridFS. Uses context.Background rather than a
// *testing.T context because this runs from t.Cleanup, where t.Context is
// already canceled.
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

func simpleMongoFilesInstanceWithMultipleFileNames(
	command string,
	fnames ...string,
) (*MongoFiles, error) {
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

	isContentSame := bytes.Equal(file1ContentsBytes, file2ContentsBytes)
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

	mf := simpleMockMongoFilesInstanceWithFilename("search", "file")
	t.Run("no args", func(t *testing.T) {
		args := []string{}
		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(t, err, "no command specified")
	})

	t.Run("non-URI positional args", func(t *testing.T) {
		for _, command := range []string{"list", "delete", "search", "get_id", "delete_id"} {
			args := []string{command, "arg1", "arg2"}
			err := mf.ValidateCommand(args)
			require.Error(t, err)
			assert.EqualError(
				t,
				err,
				"too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
				"%v: too many args",
				args,
			)
		}
	})

	t.Run("put_id with too many args", func(t *testing.T) {
		args := []string{"put_id", "arg1", "arg2", "arg3"}
		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(
			t,
			err,
			"too many non-URI positional arguments (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
			"%v: too many args",
			args,
		)
	})

	t.Run("put_id with too few args", func(t *testing.T) {
		args := []string{"put_id", "arg1"}
		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("%#q argument(s) missing", "put_id"))
	})

	t.Run("list with no args", func(t *testing.T) {
		args := []string{"list"}
		require.NoError(t, mf.ValidateCommand(args))
		assert.Equal(t, "", mf.StorageOptions.LocalFileName)
	})

	t.Run("get with multiple args", func(t *testing.T) {
		args := []string{"get", "foo", "bar", "baz"}
		require.NoError(t, mf.ValidateCommand(args))
		assert.Equal(t, []string{"foo", "bar", "baz"}, mf.FileNameList)
	})

	t.Run("put with multiple args", func(t *testing.T) {
		args := []string{"put", "foo", "bar", "baz"}
		require.NoError(t, mf.ValidateCommand(args))
		assert.Equal(t, []string{"foo", "bar", "baz"}, mf.FileNameList)
	})

	t.Run("too few args", func(t *testing.T) {
		for _, command := range []string{"get", "put", "delete", "search", "get_id", "delete_id"} {
			args := []string{command}
			err := mf.ValidateCommand(args)
			require.Error(t, err)
			assert.EqualError(
				t,
				err,
				fmt.Sprintf("%#q argument missing", command),
				"%s with no additional argument",
				command,
			)
		}
	})

	t.Run("nonsensical command", func(t *testing.T) {
		args := []string{"commandnonexistent"}

		err := mf.ValidateCommand(args)
		require.Error(t, err)
		assert.EqualError(
			t,
			err,
			fmt.Sprintf(
				"%#q is not a valid command (If you are trying to specify a connection string, it must begin with mongodb:// or mongodb+srv://)",
				args[0],
			),
		)
	})
}

// Test that the output from mongofiles is actually correct.
func TestMongoFilesCommands(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	t.Run("list command with a file that isn't in GridFS", func(t *testing.T) {
		setupGridFSTestFiles(t)
		mf, err := simpleMongoFilesInstanceWithFilename("list", "gibberish")
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		output, err := mf.Run(false)
		require.NoError(t, err, "should run the list command")
		assert.Empty(t, output, "should produce no output for a file that isn't in GridFS")
	})

	t.Run("list command with files that are in GridFS", func(t *testing.T) {
		bytesExpected := setupGridFSTestFiles(t)
		mf, err := simpleMongoFilesInstanceWithFilename("list", "testf")
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		str, err := mf.Run(false)
		require.NoError(t, err, "should run the list command")
		require.NotEmpty(t, str, "should produce some output")

		lines := cleanAndTokenizeTestOutput(str)
		require.Len(t, lines, len(testFiles), "should list every test file")

		bytesGotten := getFilesAndBytesFromLines(lines)
		assert.Equal(
			t,
			bytesExpected,
			bytesGotten,
			"should report the byte count of every test file",
		)
	})

	t.Run("search command with files that are in GridFS", func(t *testing.T) {
		bytesExpected := setupGridFSTestFiles(t)
		mf, err := simpleMongoFilesInstanceWithFilename("search", "file")
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		str, err := mf.Run(false)
		require.NoError(t, err, "should run the search command")
		require.NotEmpty(t, str, "should produce some output")

		lines := cleanAndTokenizeTestOutput(str)
		require.Len(t, lines, len(testFiles), "should find every test file")

		bytesGotten := getFilesAndBytesFromLines(lines)
		assert.Equal(
			t,
			bytesExpected,
			bytesGotten,
			"should report the byte count of every test file",
		)
	})

	t.Run("get command with a file that is in GridFS", func(t *testing.T) {
		bytesExpected := setupGridFSTestFiles(t)
		mf, err := simpleMongoFilesInstanceWithFilename("get", "testfile1")
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		var buff bytes.Buffer
		log.SetWriter(&buff)
		t.Cleanup(func() {
			removeIfExists(t, "testfile1")
			removeIfExists(t, "testfile1copy")
		})

		mf.StorageOptions.LocalFileName = "testfile1copy"
		str, err := mf.Run(false)
		require.NoError(t, err, "should run the get command")
		assert.Equal(t, "", str, "should print nothing to stdout")
		assert.NotEqual(t, 0, buff.Len(), "should log the write event")

		testFile, err := os.Open("testfile1copy")
		require.NoError(t, err, "should store the file contents under the '--local' name")
		defer testFile.Close()

		// pretty small file; so read all
		testFile1Bytes, err := io.ReadAll(testFile)
		require.NoError(t, err, "should read the retrieved file")
		assert.Len(
			t,
			testFile1Bytes,
			bytesExpected["testfile1"],
			"should retrieve the full file content",
		)
	})

	t.Run("get command with multiple files that are in GridFS", func(t *testing.T) {
		bytesExpected := setupGridFSTestFiles(t)
		localTestFiles := []string{"testfile1", "testfile2", "testfile3"}
		mf, err := simpleMongoFilesInstanceWithMultipleFileNames("get", localTestFiles...)
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		var buff bytes.Buffer
		log.SetWriter(&buff)
		t.Cleanup(func() {
			for _, testFile := range localTestFiles {
				removeIfExists(t, testFile)
			}
		})

		str, err := mf.Run(false)
		require.NoError(t, err, "should run the get command")
		require.Empty(t, str, "should print nothing to stdout")

		t.Run("log an event specifying the completion of each file", func(t *testing.T) {
			logOutput := buff.String()

			for _, testFile := range localTestFiles {
				logEvent := fmt.Sprintf("finished writing to %#q", testFile)
				assert.Contains(t, logOutput, logEvent, "should log completion of %q", testFile)
			}
		})

		t.Run("copy the files to the local filesystem", func(t *testing.T) {
			for _, testFileName := range localTestFiles {
				testFile, err := os.Open(testFileName)
				require.NoError(t, err, "should copy %q to the local filesystem", testFileName)
				defer testFile.Close()

				bytesGotten, err := io.ReadAll(testFile)
				require.NoError(t, err, "should read the retrieved file")
				assert.Len(
					t,
					bytesGotten,
					bytesExpected[testFileName],
					"should retrieve the full content of %q",
					testFileName,
				)
			}

			// Make sure that only the files that we queried
			// for are included in the local FS
			unincludedTestFile := "testfile4"
			_, err := os.Open(unincludedTestFile)
			assert.Error(t, err, "should not copy a file that wasn't requested")
		})
	})

	t.Run("get_id command with a file that is in GridFS", func(t *testing.T) {
		bytesExpected := setupGridFSTestFiles(t)
		_, err := simpleMongoFilesInstanceWithFilename("get", "testfile1")
		require.NoError(t, err, "should build a mongofiles instance")

		id := idOfFile("testfile1")
		mf, err := simpleMongoFilesInstanceWithID("get_id", id)
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		var buff bytes.Buffer
		log.SetWriter(&buff)
		t.Cleanup(func() {
			removeIfExists(t, "testfile1")
			removeIfExists(t, "testfile1copy")
		})

		str, err := mf.Run(false)
		require.NoError(t, err, "should run the get_id command")
		assert.Equal(t, "", str, "should print nothing to stdout")
		assert.NotEqual(t, 0, buff.Len(), "should log the write event")

		testFile, err := os.Open("testfile1")
		require.NoError(t, err, "should copy the file to the local filesystem")
		defer testFile.Close()

		// pretty small file; so read all
		testFile1Bytes, err := io.ReadAll(testFile)
		require.NoError(t, err, "should read the retrieved file")
		assert.Len(
			t,
			testFile1Bytes,
			bytesExpected["testfile1"],
			"should retrieve the full file content",
		)
	})

	t.Run("get_regex command with path separators", func(t *testing.T) {
		setupGridFSTestFiles(t)
		paths := []string{filepath.Join("..", "hi.txt"), filepath.Join("deepdir", "deep-hi.txt")}
		sessionProvider := setupPathTraversalFixture(t, paths)

		runPathTraversalScenarios(t, sessionProvider, paths)
	})

	t.Run(
		"get_regex command with forward slash path separators on Windows",
		func(t *testing.T) {
			if runtime.GOOS != "windows" {
				t.Skip("Windows-only path-separator scenario")
			}

			setupGridFSTestFiles(t)
			// MODIFICATION: Hardcode forward slashes instead of using filepath.Join
			paths := []string{"../hi.txt", "deepdir/deep-hi.txt"}
			sessionProvider := setupPathTraversalFixture(t, paths)

			runPathTraversalScenarios(t, sessionProvider, paths)
		},
	)

	t.Run("case insensitivity for path traversals", func(t *testing.T) {
		// Case insensitivity test is only relevant on Windows and macOS by default
		if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
			t.Skip("case-sensitivity scenario only applies on Windows and macOS")
		}

		setupGridFSTestFiles(t)

		tmpdir, err := os.MkdirTemp("", "CaseTest")
		require.NoError(t, err, "should create a temp directory")

		// 1. Create a directory with explicit mixed casing
		subdir := filepath.Join(tmpdir, "MixedCaseSubDir")
		require.NoError(t, os.Mkdir(subdir, 0o755), "should create the mixed-case subdirectory")

		cwd, err := os.Getwd()
		require.NoError(t, err, "should read the current directory")
		require.NoError(t, os.Chdir(subdir), "should change into the mixed-case subdirectory")
		t.Cleanup(
			func() { assert.NoError(t, os.Chdir(cwd), "should restore the original working directory") },
		)

		sessionProvider, err := db.NewSessionProvider(*toolOptions)
		require.NoError(t, err, "should build a session provider")

		// 2. Get the actual working directory
		currentDir, err := os.Getwd()
		require.NoError(t, err, "should read the current directory")

		// 3. Create a severely case-mismatched version of the absolute path.
		mismatchedDir := strings.ToLower(currentDir)
		if mismatchedDir == currentDir {
			// Fallback just in case the temp dir was already 100% lowercase
			mismatchedDir = strings.ToUpper(currentDir)
		}

		actualFilePath := filepath.Join(currentDir, "casetest.txt")
		mismatchedFilePath := filepath.Join(mismatchedDir, "casetest.txt")

		// 4. Create the local file
		require.NoError(
			t,
			os.WriteFile(actualFilePath, []byte("case-insensitivity test"), 0o644),
			"should write the local fixture file",
		)

		// 5. Upload it using the severely mismatched path
		putMF := MongoFiles{
			ToolOptions:     toolOptions,
			InputOptions:    &InputOptions{},
			StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
			SessionProvider: sessionProvider,
			Command:         "put",
			FileName:        mismatchedFilePath,
		}
		out, err := putMF.Run(false)
		require.NoError(t, err, "should put the file using a case-mismatched path")
		require.Empty(t, out, "should print nothing to stdout")

		// 6. Delete the local file so get_regex has to restore it
		require.NoError(
			t,
			os.Remove(actualFilePath),
			"should remove the local file before restoring it",
		)

		// restore the file without requiring --allowUnsafeTraversal
		getMF := MongoFiles{
			ToolOptions:     toolOptions,
			InputOptions:    &InputOptions{},
			StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
			SessionProvider: sessionProvider,
			Command:         "get_regex",
			FileNameRegex:   "casetest.*",
		}

		out, err = getMF.Run(false)
		require.NoError(t, err, "should restore the file without requiring --allowUnsafeTraversal")
		assert.Empty(t, out, "should print nothing to stdout")

		// Verify the tool successfully wrote the file back to disk
		_, err = os.Stat(actualFilePath)
		assert.NoError(t, err, "should write the file back to disk")
	})

	t.Run("get_regex command", func(t *testing.T) {
		setupGridFSTestFiles(t)

		cases := []struct {
			name          string
			fileNameRegex string
			regexOptions  string
			expected      map[string]struct{}
		}{
			{
				name:          "without any server options",
				fileNameRegex: "testfile[1-3]",
				expected: map[string]struct{}{
					"testfile1": {},
					"testfile2": {},
					"testfile3": {},
				},
			},
			{
				name:          "with server options",
				fileNameRegex: "tEsTfIlE[1-2]",
				regexOptions:  "i",
				expected: map[string]struct{}{
					"testfile1": {},
					"testfile2": {},
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Cleanup(func() {
					for testFile := range testFiles {
						removeIfExists(t, testFile)
					}
				})

				mf, err := simpleMongoFilesInstanceCommandOnly(GetRegex)
				require.NoError(t, err, "should build a mongofiles instance")
				mf.FileNameRegex = tc.fileNameRegex
				mf.StorageOptions.RegexOptions = tc.regexOptions

				str, err := mf.Run(false)
				require.NoError(t, err, "should run the get_regex command")
				require.Empty(t, str, "should print nothing to stdout")

				for testFile := range testFiles {
					_, err := os.Stat(testFile)
					if _, ok := tc.expected[testFile]; ok {
						assert.NoError(t, err, "should restore %q", testFile)
					} else {
						assert.Error(t, err, "should not restore %q", testFile)
					}
				}
			})
		}
	})

	t.Run("put command with multiple lorem ipsum files", func(t *testing.T) {
		setupGridFSTestFiles(t)
		localTestFiles := []string{
			filepath.FromSlash("testdata/lorem_ipsum_multi_args_0.txt"),
			filepath.FromSlash("testdata/lorem_ipsum_multi_args_1.txt"),
			filepath.FromSlash("testdata/lorem_ipsum_multi_args_2.txt"),
		}

		mf, err := simpleMongoFilesInstanceWithMultipleFileNames("put", localTestFiles...)
		require.NoError(t, err, "should build a mongofiles instance")

		var buff bytes.Buffer
		log.SetWriter(&buff)

		str, err := mf.Run(false)
		require.NoError(t, err, "should run the put command")
		require.Empty(t, str, "should print nothing to stdout")

		t.Run("log an event specifying the completion of each file", func(t *testing.T) {
			const (
				logAdding = "adding gridFile: %#q"
				logAdded  = "added gridFile: %#q"
			)

			logOutput := buff.String()

			for _, testFile := range localTestFiles {
				assert.Contains(
					t,
					logOutput,
					fmt.Sprintf(logAdding, testFile),
					"should log adding %q",
					testFile,
				)
				assert.Contains(
					t,
					logOutput,
					fmt.Sprintf(logAdded, testFile),
					"should log completion of %q",
					testFile,
				)
			}
		})

		t.Run("and files should exist in GridFS", func(t *testing.T) {
			bytesGotten, err := getFilesAndBytesListFromGridFS()
			require.NoError(t, err, "should list the files in GridFS")

			// Check that the only files included are the local test
			// files, i.e. in localTestFiles, and the global test
			// files, i.e. in testFiles
			assert.Len(
				t,
				bytesGotten,
				len(localTestFiles)+len(testFiles),
				"should list every local and fixture test file",
			)

			for _, testFile := range localTestFiles {
				assert.Contains(t, bytesGotten, testFile, "should list %q", testFile)
			}
		})

		t.Run(
			"and each file should have exactly the same content as the original file",
			func(t *testing.T) {
				const localFileName = "lorem_ipsum_copy.txt"
				t.Cleanup(
					func() { assert.NoError(t, os.Remove(localFileName), "should remove the retrieved copy") },
				)

				for i, testFile := range localTestFiles {
					mfAfter, err := simpleMongoFilesInstanceWithFilename("get", testFile)
					require.NoError(t, err, "should build a mongofiles instance")
					require.NotNil(t, mf, "should build a mongofiles instance")

					mfAfter.StorageOptions.LocalFileName = localFileName

					if i > 0 {
						_, err = mfAfter.Run(false)
						require.Error(t, err, "should refuse to overwrite an existing local file")
						require.ErrorIs(
							t,
							err,
							os.ErrExist,
							"should report the local file as already existing",
						)
					}

					mfAfter.StorageOptions.OverwriteLocal = true
					str, err := mfAfter.Run(false)
					require.NoError(t, err, "should overwrite the local file when asked to")
					require.Empty(t, str, "should print nothing to stdout")

					isContentSame := compareToOriginalLoremIpsum(t, testFile, localFileName)
					assert.True(
						t,
						isContentSame,
						"should retrieve %q with unchanged content",
						testFile,
					)
				}
			},
		)
	})

	t.Run("put_id command with different ids", func(t *testing.T) {
		setupGridFSTestFiles(t)
		for _, idToTest := range []string{`test_id`, `{"a":"b"}`, `{"$numberLong":"999999999999999"}`, `{"a":{"b":{"c":{}}}}`} {
			runPutIDTestCase(t, idToTest)
		}
	})

	t.Run("delete command with a file that is in GridFS", func(t *testing.T) {
		setupGridFSTestFiles(t)
		mf, err := simpleMongoFilesInstanceWithFilename("delete", "testfile2")
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		var buff bytes.Buffer
		log.SetWriter(&buff)

		str, err := mf.Run(false)
		require.NoError(t, err, "should delete the file from GridFS")
		assert.Equal(t, "", str, "should print nothing to stdout")
		assert.NotEqual(t, 0, buff.Len(), "should log the deletion")

		bytesGotten, err := getFilesAndBytesListFromGridFS()
		require.NoError(t, err, "should list the remaining files in GridFS")
		assert.Len(t, bytesGotten, len(testFiles)-1, "should delete exactly one file from GridFS")
		assert.NotContains(
			t,
			bytesGotten,
			"testfile2",
			"should remove the deleted file from GridFS",
		)
	})

	t.Run("delete_id command with a file that is in GridFS", func(t *testing.T) {
		setupGridFSTestFiles(t)
		// hack to grab an _id
		_, err := simpleMongoFilesInstanceWithFilename("get", "testfile2")
		require.NoError(t, err, "should build a mongofiles instance")

		idString := idOfFile("testfile2")
		mf, err := simpleMongoFilesInstanceWithID("delete_id", idString)
		require.NoError(t, err, "should build a mongofiles instance")
		require.NotNil(t, mf, "should build a mongofiles instance")

		var buff bytes.Buffer
		log.SetWriter(&buff)

		str, err := mf.Run(false)
		require.NoError(t, err, "should delete the file from GridFS")
		assert.Equal(t, "", str, "should print nothing to stdout")
		assert.NotEqual(t, 0, buff.Len(), "should log the deletion")

		bytesGotten, err := getFilesAndBytesListFromGridFS()
		require.NoError(t, err, "should list the remaining files in GridFS")
		assert.Len(t, bytesGotten, len(testFiles)-1, "should delete exactly one file from GridFS")
		assert.NotContains(
			t,
			bytesGotten,
			"testfile2",
			"should remove the deleted file from GridFS",
		)
	})
}

// runPutIDTestCase inserts a lorem-ipsum file under idToTest and checks that
// it round-trips through GridFS unchanged. Uses require throughout, matching
// the original GoConvey FailureHalts semantics: a failure on one id aborts
// the remaining ids rather than running them independently, since the
// original ran all four ids in a single (unlooped) Convey leaf.
func runPutIDTestCase(t *testing.T, idToTest string) {
	t.Helper()

	remoteName := "remoteName"
	mongoFilesInstance, err := simpleMongoFilesInstanceWithFilenameAndID(
		"put_id",
		remoteName,
		idToTest,
	)

	var buff bytes.Buffer
	log.SetWriter(&buff)

	require.NoError(t, err, "should build a mongofiles instance")
	require.NotNil(t, mongoFilesInstance, "should build a mongofiles instance")
	mongoFilesInstance.StorageOptions.LocalFileName = filepath.FromSlash(
		"testdata/lorem_ipsum_287613_bytes.txt",
	)

	t.Log("should correctly insert the file into GridFS")
	str, err := mongoFilesInstance.Run(false)
	require.NoError(t, err, "should run the put_id command")
	require.Equal(t, "", str, "should print nothing to stdout")
	require.NotEqual(t, 0, buff.Len(), "should log the write event")

	t.Log("and its filename should exist when the 'list' command is run")
	bytesGotten, err := getFilesAndBytesListFromGridFS()
	require.NoError(t, err, "should list the files in GridFS")
	require.Contains(t, bytesGotten, remoteName, "should list the file under its remote name")

	t.Log("and get_id should have exactly the same content as the original file")

	mfAfter, err := simpleMongoFilesInstanceWithID("get_id", idToTest)
	require.NoError(t, err, "should build a mongofiles instance")
	require.NotNil(t, mfAfter, "should build a mongofiles instance")

	mfAfter.StorageOptions.LocalFileName = "lorem_ipsum_copy.txt"
	mfAfter.StorageOptions.OverwriteLocal = true
	buff.Truncate(0)
	str, err = mfAfter.Run(false)
	require.NoError(t, err, "should retrieve the file by its id")
	require.Equal(t, "", str, "should print nothing to stdout")
	require.NotEqual(t, 0, buff.Len(), "should log the write event")

	loremIpsumOrig, err := os.Open(filepath.FromSlash("testdata/lorem_ipsum_287613_bytes.txt"))
	require.NoError(t, err, "should open the original file")

	loremIpsumCopy, err := os.Open("lorem_ipsum_copy.txt")
	require.NoError(t, err, "should open the retrieved copy")

	defer loremIpsumOrig.Close()
	defer loremIpsumCopy.Close()

	isContentSame, err := fileContentsCompare(loremIpsumOrig, loremIpsumCopy, t)
	require.NoError(t, err, "should compare the two files")
	require.True(t, isContentSame, "should retrieve the file with unchanged content")
}

// compareToOriginalLoremIpsum compares the retrieved copy against the
// original file, closing both before returning so each iteration of the
// caller's loop doesn't accumulate open handles across files.
func compareToOriginalLoremIpsum(t *testing.T, original, copyName string) bool {
	t.Helper()

	loremIpsumOrig, err := os.Open(original)
	require.NoError(t, err, "should open the original file %q", original)
	defer loremIpsumOrig.Close()

	loremIpsumCopy, err := os.Open(copyName)
	require.NoError(t, err, "should open the retrieved copy of %q", original)
	defer loremIpsumCopy.Close()

	isContentSame, err := fileContentsCompare(loremIpsumOrig, loremIpsumCopy, t)
	require.NoError(t, err, "should compare the two files")

	return isContentSame
}

// setupGridFSTestFiles seeds GridFS with the fixture files every subtest
// exercises and registers the teardown that mirrors the original outer
// Reset, which fired after every leaf: drop the test database and remove
// any leftover local copy file.
func setupGridFSTestFiles(t *testing.T) map[string]int {
	t.Helper()

	bytesExpected, err := setUpGridFSTestData()
	require.NoError(t, err, "should seed GridFS with the test fixture files")

	t.Cleanup(func() {
		assert.NoError(t, tearDownGridFSTestData(), "should tear down the GridFS test database")
		_ = os.Remove("lorem_ipsum_copy.txt")
	})

	return bytesExpected
}

// removeIfExists mirrors the original Reset bodies that only asserted a
// remove call when the file was actually present.
func removeIfExists(t *testing.T, name string) {
	t.Helper()

	if fileExists(name) {
		assert.NoError(t, os.Remove(name), "should remove %q", name)
	}
}

// setupPathTraversalFixture builds the nested directory used by the
// get_regex path-separator scenarios and puts each path into GridFS once.
// Safe to share across the sibling scenarios that follow: none of them
// mutate this fixture, they only restore into it and clean up after
// themselves.
func setupPathTraversalFixture(t *testing.T, paths []string) *db.SessionProvider {
	t.Helper()

	tmpdir, err := os.MkdirTemp("", "")
	require.NoError(t, err, "should create a temp directory")

	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpdir, "hi.txt"), []byte("hi"), 0o644),
		"should write the parent-directory fixture file",
	)

	subdir := filepath.Join(tmpdir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o755), "should create the working subdirectory")

	cwd, err := os.Getwd()
	require.NoError(t, err, "should read the current directory")
	require.NoError(t, os.Chdir(subdir), "should change into the working subdirectory")
	t.Cleanup(
		func() { assert.NoError(t, os.Chdir(cwd), "should restore the original working directory") },
	)

	require.NoError(t, os.Mkdir("deepdir", 0o755), "should create the nested directory")
	require.NoError(
		t,
		os.WriteFile(filepath.Join("deepdir", "deep-hi.txt"), []byte("deep-hi"), 0o644),
		"should write the nested fixture file",
	)

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	require.NoError(t, err, "should build a session provider")

	for _, path := range paths {
		putMF := MongoFiles{
			ToolOptions:     toolOptions,
			InputOptions:    &InputOptions{},
			StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
			SessionProvider: sessionProvider,
			Command:         "put",
			FileName:        path,
		}
		out, err := putMF.Run(false)
		require.NoError(t, err, "should put %q into GridFS", path)
		require.Empty(t, out, "should print nothing to stdout")

		require.NoError(t, os.Remove(path), "should remove the local copy of %q after upload", path)
	}

	return sessionProvider
}

// runPathTraversalScenarios exercises the three get_regex traversal
// scenarios shared by the plain and Windows-forward-slash variants.
func runPathTraversalScenarios(t *testing.T, sessionProvider *db.SessionProvider, paths []string) {
	t.Helper()

	t.Run("forbid unsafe traversals by default", func(t *testing.T) {
		getMF := MongoFiles{
			ToolOptions:     toolOptions,
			InputOptions:    &InputOptions{},
			StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
			SessionProvider: sessionProvider,
			Command:         "get_regex",
			FileNameRegex:   ".*",
		}

		_, err := getMF.Run(false)
		require.Error(t, err, "should reject an unsafe traversal without opt-in")
		assert.Contains(
			t,
			fmt.Sprint(err),
			"--allowUnsafeTraversal",
			"should mention the opt-in flag",
		)

		for _, path := range paths {
			_, err := os.Stat(path)
			require.Error(t, err, "should not restore %q without opt-in", path)
			require.ErrorIs(t, err, os.ErrNotExist, "should report %q as missing", path)
		}
	})

	t.Run("allow deep restores under the current directory", func(t *testing.T) {
		t.Cleanup(
			func() { assert.NoError(t, os.RemoveAll(paths[1]), "should clean up the restored file") },
		)

		getMF := MongoFiles{
			ToolOptions:     toolOptions,
			InputOptions:    &InputOptions{},
			StorageOptions:  &StorageOptions{GridFSPrefix: "fs", DB: testDB},
			SessionProvider: sessionProvider,
			Command:         "get_regex",
			FileNameRegex:   "deep*",
		}
		out, err := getMF.Run(false)
		require.NoError(t, err, "should restore a file under the current directory")
		assert.Empty(t, out, "should print nothing to stdout")

		_, err = os.Stat(paths[1])
		assert.NoError(t, err, "should restore %q", paths[1])
	})

	t.Run("allow unsafe traversals by opt-in", func(t *testing.T) {
		t.Cleanup(func() {
			for _, path := range paths {
				assert.NoError(t, os.RemoveAll(path), "should clean up %q", path)
			}
		})

		getMF := MongoFiles{
			ToolOptions:  toolOptions,
			InputOptions: &InputOptions{},
			StorageOptions: &StorageOptions{
				GridFSPrefix:         "fs",
				DB:                   testDB,
				AllowUnsafeTraversal: true,
			},
			SessionProvider: sessionProvider,
			Command:         "get_regex",
			FileNameRegex:   ".*",
		}

		out, err := getMF.Run(false)
		require.NoError(t, err, "should restore files when unsafe traversal is allowed")
		assert.Empty(t, out, "should print nothing to stdout")

		for _, path := range paths {
			_, err := os.Stat(path)
			assert.NoError(t, err, "should restore %q", path)
		}
	})
}

// Test that when no write concern is specified, a majority write concern is set.
func TestDefaultWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	if ssl.UseSSL {
		t.Skip("Skipping non-SSL test with SSL configuration")
	}

	t.Run("with a URI that doesn't specify write concern", func(t *testing.T) {
		mf, err := getMongofilesWithArgs("get", "filename", "--uri", "mongodb://localhost:33333")
		require.NoError(t, err, "should build mongofiles from a URI")
		assert.Equal(
			t,
			writeconcern.Majority(),
			mf.ToolOptions.WriteConcern,
			"should default to majority write concern",
		)
	})

	t.Run("with no URI and no write concern option", func(t *testing.T) {
		mf, err := getMongofilesWithArgs("get", "filename", "--port", "33333")
		require.NoError(t, err, "should build mongofiles from a port")
		assert.Equal(
			t,
			writeconcern.Majority(),
			mf.ToolOptions.WriteConcern,
			"should default to majority write concern",
		)
	})
}
