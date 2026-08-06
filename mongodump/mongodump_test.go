// copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/failpoint"
	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/xoptions"
)

var (
	// database with test data.
	testDB = "mongodump_test_db"
	// temp database used for restoring a DB.
	testRestoreDB       = "temp_mongodump_restore_test_db"
	testCollectionNames = []string{"coll1", "coll2", "coll/three"}
)

const (
	longPrefix = "aVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery" +
		"VeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery" +
		"VeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery"
	longCollectionName = longPrefix + "/Long/Collection/Name/Consisting/Of/Many/Characters"
	longBsonName       = longPrefix + "%2FLong%2FCollection%2FName%24FUVlwTrb2eHN1RUE1swI1fFzWmA.bson"
	longMetadataName   = longPrefix + "%2FLong%2FCollection%2FName%24FUVlwTrb2eHN1RUE1swI1fFzWmA.metadata.json"
)

func simpleMongoDumpInstance() (*MongoDump, error) {
	toolOptions, err := testutil.GetToolOptions()
	if err != nil {
		return nil, fmt.Errorf("error getting tool options to create a mongodump instance: %w", err)
	}

	// Limit ToolOptions to test database
	toolOptions.Namespace = &options.Namespace{DB: testDB}

	outputOptions := &OutputOptions{
		NumParallelCollections: 1,
	}
	inputOptions := &InputOptions{}

	log.SetVerbosity(toolOptions.Verbosity)

	return &MongoDump{
		ToolOptions:   toolOptions,
		InputOptions:  inputOptions,
		OutputOptions: outputOptions,
	}, nil
}

// returns the number of .bson files in a directory
// excluding system.indexes.bson.
func countNonIndexBSONFiles(dir string) (int, error) {
	files, err := listNonIndexBSONFiles(dir)
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

func assertBSONEqual(t *testing.T, expected, actual any) {
	expectedJSON, err := bson.MarshalExtJSONIndent(expected, false, false, "", "    ")
	require.NoError(t, err)

	actualJSON, err := bson.MarshalExtJSONIndent(actual, false, false, "", "    ")
	require.NoError(t, err)

	assert.Equal(t, string(expectedJSON), string(actualJSON))
}

func listNonIndexBSONFiles(dir string) ([]string, error) {
	var files []string
	matchingFiles, err := getMatchingFiles(dir, ".*\\.bson")
	if err != nil {
		return nil, err
	}
	for _, fileName := range matchingFiles {
		if fileName != "system.indexes.bson" {
			files = append(files, fileName)
		}
	}
	return files, nil
}

// returns count of metadata files.
func countMetaDataFiles(dir string) (int, error) {
	matchingFiles, err := getMatchingFiles(dir, ".*\\.metadata\\.json")
	if err != nil {
		return 0, err
	}
	return len(matchingFiles), nil
}

// returns count of oplog entries with 'ui' field.
func countOplogUI(iter *db.DecodedBSONSource) int {
	var count int
	var doc bson.M
	for iter.Next(&doc) {
		count += countOpsWithUI(doc)
	}
	return count
}

func countOpsWithUI(doc bson.M) int {
	var count int
	switch doc["op"] {
	case "i", "u", "d":
		if _, ok := doc["ui"]; ok {
			count++
		}
	case "c":
		if _, ok := doc["ui"]; ok {
			count++
		} else if v, ok := doc["o"]; ok {
			opts, _ := v.(bson.M)
			if applyOps, ok := opts["applyOps"]; ok {
				//nolint:errcheck
				list := applyOps.([]bson.M)
				for _, v := range list {
					count += countOpsWithUI(v)
				}
			}
		}
	}
	return count
}

// returns filenames that match the given pattern.
func getMatchingFiles(dir, pattern string) ([]string, error) {
	fileInfos, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	matchingFiles := []string{}
	var matched bool
	for _, fileInfo := range fileInfos {
		fileName := fileInfo.Name()
		if matched, err = regexp.MatchString(pattern, fileName); matched {
			matchingFiles = append(matchingFiles, fileName)
		}
		if err != nil {
			return nil, err
		}
	}
	return matchingFiles, nil
}

// read all the database bson documents from dir and put it into another DB
// ignore the indexes for now.
func readBSONIntoDatabase(t *testing.T, dir, restoreDBName string) error {
	if ok := fileDirExists(dir); !ok {
		return fmt.Errorf("error finding '%v' on local FS", dir)
	}

	session, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	fileInfos, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, fileInfo := range fileInfos {
		fileName := fileInfo.Name()
		if !strings.HasSuffix(fileName, ".bson") || fileName == "system.indexes.bson" {
			continue
		}

		collectionName, err := url.QueryUnescape(
			fileName[:strings.LastIndex(fileName, ".bson")],
		)
		if err != nil {
			return err
		}

		collection := session.Database(restoreDBName).Collection(collectionName)

		file, err := os.Open(fmt.Sprintf("%s/%s", dir, fileName))
		if err != nil {
			return err
		}
		defer file.Close()

		bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(file))
		defer bsonSource.Close()

		var result bson.D
		for bsonSource.Next(&result) {
			_, err = collection.InsertOne(t.Context(), result)
			if err != nil {
				return err
			}
		}
		if err = bsonSource.Err(); err != nil {
			return err
		}
	}

	return nil
}

func setUpMongoDumpTestData(t *testing.T) error {
	session, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	for i, collectionName := range testCollectionNames {
		coll := session.Database(testDB).Collection(collectionName)

		for j := 0; j < 10*(i+1); j++ {
			_, err = coll.InsertOne(
				t.Context(),
				bson.M{
					"collectionName": collectionName,
					"age":            j,
					"":               "foo",
					"coords":         bson.D{{"x", i}, {"y", j}},
				},
			)
			if err != nil {
				return err
			}

			idx := mongo.IndexModel{
				Keys: bson.M{`"`: 1},
			}
			_, err = coll.Indexes().CreateOne(t.Context(), idx)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func setupTimeseriesWithMixedSchema(t *testing.T, dbName string, collName string) {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "get server version")

	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "get session")

	err = client.Database(dbName).Collection(collName).Drop(t.Context())
	require.NoError(t, err, "drop existing coll")

	createCmd := bson.D{
		{"create", collName},
		{"timeseries", bson.D{
			{"timeField", "t"},
			{"metaField", "m"},
		}},
	}

	createRes := sessionProvider.DB(dbName).RunCommand(t.Context(), createCmd)
	require.NoError(t, createRes.Err(), "create timeseries coll")

	// SERVER-84531 was only backported to 7.3.
	// TODO: Run collMod command on 6.0 and 7.0 (TOOLS-3597).
	clientFCV := testutil.GetFCV(client)

	shouldAccommodateMixedSchema := clientFCV == "6.0" || clientFCV == "7.0"
	if !shouldAccommodateMixedSchema {
		cmp, err := testutil.CompareFCV(clientFCV, "7.3")
		require.NoError(t, err, "compare client fcv (%s)", clientFCV)

		shouldAccommodateMixedSchema = cmp >= 0
	}

	if shouldAccommodateMixedSchema {
		res := sessionProvider.DB(dbName).RunCommand(t.Context(), bson.D{
			{"collMod", collName},
			{"timeseriesBucketsMayHaveMixedSchemaData", true},
		})
		require.NoError(t, res.Err(), "set mixed schema data")
	}

	bucketName := timeseriesCollName(serverVersion, collName)
	bucketColl := sessionProvider.DB(dbName).Collection(bucketName)
	bucketJSON := `{"_id":{"$oid":"65a6eb806ffc9fa4280ecac4"},"control":{"version":1,"min":{"_id":{"$oid":"65a6eba7e6d2e848e08c3750"},"t":{"$date":"2024-01-16T20:48:00Z"},"a":1},"max":{"_id":{"$oid":"65a6eba7e6d2e848e08c3751"},"t":{"$date":"2024-01-16T20:48:39.448Z"},"a":"a"}},"meta":0,"data":{"_id":{"0":{"$oid":"65a6eba7e6d2e848e08c3750"},"1":{"$oid":"65a6eba7e6d2e848e08c3751"}},"t":{"0":{"$date":"2024-01-16T20:48:39.448Z"},"1":{"$date":"2024-01-16T20:48:39.448Z"}},"a":{"0":"a","1":1}}}`

	var bucketDoc bson.D
	err = bson.UnmarshalExtJSON([]byte(bucketJSON), false, &bucketDoc)
	require.NoError(t, err, "unmarshal ext json")

	opts := mopt.InsertOne()
	if serverVersion.SupportsRawData() {
		err := xoptions.SetInternalInsertOneOptions(opts, "rawData", true)
		require.NoError(t, err)
	}

	_, err = bucketColl.InsertOne(t.Context(), bucketDoc, opts)
	require.NoError(t, err, "insert bucket doc")
}

func timeseriesCollName(version db.Version, base string) string {
	if version.SupportsRawData() {
		// viewless timeseries
		return base
	}

	return common.TimeseriesBucketPrefix + base
}

func setUpDBView(dbName string, colName string) error {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		return err
	}

	pipeline := []bson.M{{"$project": bson.M{"b": "$a"}}}
	createCmd := bson.D{
		{"create", "test view"},
		{"viewOn", colName},
		{"pipeline", pipeline},
	}
	var r2 bson.D
	err = sessionProvider.Run(createCmd, &r2, dbName)
	if err != nil {
		return err
	}
	return nil
}

func turnOnProfiling(dbName string) error {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		return err
	}

	profileCmd := bson.D{
		{"profile", 2},
	}

	var res bson.M
	return sessionProvider.Run(profileCmd, &res, dbName)
}

func countSnapshotCmds(
	t *testing.T,
	profileCollection *mongo.Collection,
	ns string,
) (int64, error) {
	return profileCollection.CountDocuments(t.Context(),
		bson.D{
			{"ns", ns},
			{"op", "query"},
			{"$or", []any{
				// 4.0+
				bson.D{{"command.hint._id", 1}},
				// 3.6
				bson.D{{"command.$snapshot", true}},
				bson.D{{"command.snapshot", true}},
				// 3.4 and previous
				bson.D{{"query.$snapshot", true}},
				bson.D{{"query.snapshot", true}},
				bson.D{{"query.hint._id", 1}},
			}},
		},
	)
}

// backgroundInsert inserts into random collections until provided done
// channel is closed.  The function closes the ready channel to signal that
// background insertion has started.  When the done channel is closed, the
// function returns.  Any errors are passed back on the errs channel.
func backgroundInsert(t *testing.T, ready, done chan struct{}, errs chan error) {
	defer close(errs)
	session, err := testutil.GetBareSession()
	if err != nil {
		errs <- err
		close(ready)
		return
	}

	colls := make([]*mongo.Collection, len(testCollectionNames))
	for i, v := range testCollectionNames {
		colls[i] = session.Database(testDB).Collection(v)
	}

	var n int

	// Insert a doc to ensure the DB is actually ready for inserts
	// and not pausing while a dropDatabase is processing.
	_, err = colls[0].InsertOne(t.Context(), bson.M{"n": n})
	if err != nil {
		errs <- err
		close(ready)
		return
	}
	close(ready)
	n++

	for {
		select {
		case <-done:
			return
		default:
			coll := colls[rand.Intn(len(colls))]
			_, err := coll.InsertOne(t.Context(), bson.M{"n": n})
			if err != nil {
				errs <- err
				return
			}
			n++
		}
	}
}

func tearDownMongoDumpTestData(t *testing.T) error {
	session, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	err = session.Database(testDB).Drop(t.Context())
	if err != nil {
		return err
	}
	return nil
}

// tearDownMongoDumpTestDataInCleanup mirrors tearDownMongoDumpTestData for use
// inside a t.Cleanup body, where t.Context() is already canceled.
func tearDownMongoDumpTestDataInCleanup() error {
	session, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	return session.Database(testDB).Drop(context.Background())
}

func dropDB(t *testing.T, dbName string) error {
	session, err := testutil.GetBareSession()
	if err != nil {
		return err
	}

	err = session.Database(dbName).Drop(t.Context())
	if err != nil {
		return err
	}
	return nil
}

func fileDirExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// testQuery dumps each of testCollectionNames using --query* filters already
// set on md, restores the dump, and checks that only the filtered documents
// came back. It shares one restore database across every collection, so
// callers own tearing that database down.
func testQuery(t *testing.T, md *MongoDump, session *mongo.Client) string {
	t.Helper()

	origDB := session.Database(testDB)
	restoredDB := session.Database(testRestoreDB)

	// query to test --query* flags
	bsonQuery := bson.M{"age": bson.M{"$lt": 10}}

	// we can only dump using query per collection
	for _, testCollName := range testCollectionNames {
		md.ToolOptions.Collection = testCollName

		require.NoError(t, md.Init(), "should initialize mongodump for %q", testCollName)
		require.NoError(t, md.Dump(), "should dump %q with the query applied", testCollName)
	}

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
	dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
	require.True(t, fileDirExists(dumpDir), "should create the dump directory")
	require.True(t, fileDirExists(dumpDBDir), "should create the database directory")

	require.NoError(
		t,
		restoredDB.Drop(t.Context()),
		"should drop any pre-existing restore database",
	)
	require.NoError(
		t,
		readBSONIntoDatabase(t, dumpDBDir, testRestoreDB),
		"should restore the dumped bson into the database",
	)

	for _, testCollName := range testCollectionNames {
		// count filtered docs
		origDocCount, err := origDB.Collection(testCollName).
			CountDocuments(t.Context(), bsonQuery)
		require.NoError(t, err, "should count the original filtered documents for %q", testCollName)

		// count number of all restored documents
		restDocCount, err := restoredDB.Collection(testCollName).
			CountDocuments(t.Context(), bson.D{})
		require.NoError(t, err, "should count the restored documents for %q", testCollName)

		require.EqualValues(
			t,
			origDocCount,
			restDocCount,
			"should restore exactly the documents matching the query for %q",
			testCollName,
		)
	}
	return dumpDir
}

// testDumpOneCollection dumps md's configured collection to dumpDir, restores
// it, and checks the restored collection matches the original. The two
// checks below (count, then per-document content) read the same restored
// data and don't need independent setup, so they run as one pass rather than
// as separate scenarios.
func testDumpOneCollection(t *testing.T, md *MongoDump, dumpDir string) {
	t.Helper()

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	absDumpDir := filepath.FromSlash(filepath.Join(path, dumpDir))
	require.NoError(t, os.RemoveAll(absDumpDir), "should remove any pre-existing dump directory")
	require.False(t, fileDirExists(absDumpDir), "should not have a dump directory before dumping")

	dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
	require.False(
		t,
		fileDirExists(dumpDBDir),
		"should not have a database directory before dumping",
	)

	md.OutputOptions.Out = dumpDir
	require.NoError(t, md.Dump(), "should dump the collection")
	require.True(t, fileDirExists(dumpDBDir), "should create the database directory")

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "should connect to the server")

	countColls, err := countNonIndexBSONFiles(dumpDBDir)
	require.NoError(t, err, "should count the dumped bson files")
	require.EqualValues(t, 1, countColls, "should dump exactly one collection")

	collOriginal := session.Database(testDB).Collection(md.ToolOptions.Collection)

	require.NoError(
		t,
		session.Database(testRestoreDB).Drop(t.Context()),
		"should drop any pre-existing restore database",
	)
	collRestore := session.Database(testRestoreDB).Collection(md.ToolOptions.Collection)

	require.NoError(
		t,
		readBSONIntoDatabase(t, dumpDBDir, testRestoreDB),
		"should restore the dumped bson into the database",
	)

	// with the correct number of documents
	numDocsOrig, err := collOriginal.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err, "should count the original documents")

	numDocsRestore, err := collRestore.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err, "should count the restored documents")

	require.EqualValues(
		t,
		numDocsOrig,
		numDocsRestore,
		"should restore the correct number of documents",
	)

	// that are the same as the documents in the test database
	iter, err := collOriginal.Find(t.Context(), bson.D{})
	require.NoError(t, err, "should query the original collection")

	var result bson.D
	for iter.Next(t.Context()) {
		require.NoError(t, iter.Decode(&result), "should decode the original document")
		restoredCount, err := collRestore.CountDocuments(t.Context(), result)
		require.NoError(t, err, "should count matching restored documents")
		require.NotZero(
			t,
			restoredCount,
			"should find each original document in the restored collection",
		)
	}
	require.NoError(t, iter.Err(), "should iterate every original document without error")
	require.NoError(t, iter.Close(t.Context()), "should close the original cursor")
}

func TestMongoDumpValidateOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("no db", func(t *testing.T) {
		md, err := simpleMongoDumpInstance()
		require.NoError(t, err)
		md.ToolOptions.Collection = "some_collection"
		md.ToolOptions.DB = ""

		err = md.ValidateOptions()
		require.Error(t, err)
		assert.ErrorContains(
			t,
			err,
			"cannot dump a collection without a specified database",
		)
	})

	t.Run("no collection name with query", func(t *testing.T) {
		md, err := simpleMongoDumpInstance()
		require.NoError(t, err)

		md.ToolOptions.Collection = ""
		md.InputOptions.Query = "{_id:\"\"}"

		err = md.ValidateOptions()
		require.Error(t, err)
		assert.ErrorContains(
			t,
			err,
			"cannot dump using a query without a specified collection",
		)
	})
}

func TestMongoDumpConnectedToAtlasProxy(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)
	defer sessionProvider.Close()

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	md.isAtlasProxy = true
	md.OutputOptions.DumpDBUsersAndRoles = false
	md.SessionProvider = sessionProvider

	session, err := sessionProvider.GetSession()
	require.NoError(t, err)
	// This case shouldn't error and should instead not return that it will try to restore users and roles.
	_, err = session.Database("admin").
		Collection("testcol").
		InsertOne(t.Context(), bson.M{})
	require.NoError(t, err)
	dbNames, err := md.GetValidDbs()
	require.NoError(t, err)
	require.NotContains(t, dbNames, "admin")

	// This case should error because it has explicitly been set to dump users and roles for a DB, but thats
	// not possible with an atlas proxy.
	md.OutputOptions.DumpDBUsersAndRoles = true
	err = md.ValidateOptions()
	require.Error(
		t,
		err,
		"can't dump from admin database when connecting to a MongoDB Atlas free or shared cluster",
	)

	require.NoError(t, session.Database("admin").Collection("testcol").Drop(t.Context()))
}

func TestMongoDumpBSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	t.Run("dumps a particular collection to the default output directory", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		md := newMongoDumpForBSONSubtest(t, testCollectionNames[0])
		require.NoError(t, md.Init(), "should initialize mongodump")
		testDumpOneCollection(t, md, "dump")
	})

	t.Run("dumps a particular collection to a user-specified output directory", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		md := newMongoDumpForBSONSubtest(t, testCollectionNames[0])
		require.NoError(t, md.Init(), "should initialize mongodump")
		testDumpOneCollection(t, md, "dump_user")
	})

	t.Run("dumps a particular collection to standard output", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		md := newMongoDumpForBSONSubtest(t, testCollectionNames[0])
		require.NoError(t, md.Init(), "should initialize mongodump")

		md.OutputOptions.Out = "-"
		stdoutBuf := &bytes.Buffer{}
		md.OutputWriter = stdoutBuf
		require.NoError(t, md.Dump(), "should dump the collection to standard output")

		var count int
		bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(io.NopCloser(stdoutBuf)))
		defer bsonSource.Close()

		var result bson.Raw
		for bsonSource.Next(&result) {
			count++
		}
		require.NoError(t, bsonSource.Err(), "should read every dumped document")
		// The 0th collection has 10 documents.
		require.EqualValues(t, 10, count, "should dump every document in the collection")
	})

	t.Run("dumps a collection with a slash in its name to the filesystem", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		md := newMongoDumpForBSONSubtest(t, testCollectionNames[2])
		require.NoError(t, md.Init(), "should initialize mongodump")
		testDumpOneCollection(t, md, "dump_slash")
	})

	t.Run(
		"initializes a collection with a slash in its name for archive output",
		func(t *testing.T) {
			setUpMongoDumpBSONSubtest(t)
			md := newMongoDumpForBSONSubtest(t, testCollectionNames[2])
			md.OutputOptions.Archive = "dump_slash.archive"
			require.NoError(t, md.Init(), "should initialize mongodump for archive output")
		},
	)

	t.Run(
		"dumps an entire database that exists, producing bson files for every collection",
		func(t *testing.T) {
			setUpMongoDumpBSONSubtest(t)
			md := newMongoDumpForBSONSubtest(t, "")
			require.NoError(t, md.Init(), "should initialize mongodump")

			md.OutputOptions.Out = "dump"
			require.NoError(t, md.Dump(), "should dump the whole database")

			path, err := os.Getwd()
			require.NoError(t, err, "should get the working directory")

			dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
			dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
			require.True(t, fileDirExists(dumpDir), "should create the dump directory")
			require.True(t, fileDirExists(dumpDBDir), "should create the database directory")

			countColls, err := countNonIndexBSONFiles(dumpDBDir)
			require.NoError(t, err, "should count the dumped bson files")
			require.EqualValues(
				t,
				len(testCollectionNames),
				countColls,
				"should dump every collection",
			)

			t.Cleanup(func() {
				assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
			})
		},
	)

	t.Run(
		"does not create a dump directory for a database that does not exist",
		func(t *testing.T) {
			setUpMongoDumpBSONSubtest(t)
			md := newMongoDumpForBSONSubtest(t, "")
			require.NoError(t, md.Init(), "should initialize mongodump")

			md.OutputOptions.Out = "dump"
			md.ToolOptions.DB = "nottestdb"
			require.NoError(t, md.Dump(), "should succeed dumping a nonexistent database")

			path, err := os.Getwd()
			require.NoError(t, err, "should get the working directory")

			dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
			dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, "nottestdb"))
			require.False(t, fileDirExists(dumpDir), "should not create the dump directory")
			require.False(
				t,
				fileDirExists(dumpDBDir),
				"should not create the database directory",
			)
		},
	)

	t.Run("using --query for all the collections in the database", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		session, jsonQueryBytes := newMongoDumpQuerySubtestFixture(t)
		md := newMongoDumpForBSONSubtest(t, "")

		md.InputOptions.Query = string(jsonQueryBytes)
		md.ToolOptions.DB = testDB
		md.OutputOptions.Out = "dump"
		dumpDir := testQuery(t, md, session)

		t.Cleanup(func() {
			assert.NoError(
				t,
				session.Database(testRestoreDB).Drop(context.Background()),
				"should drop the restore database",
			)
			assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
		})
	})

	t.Run("using --queryFile for all the collections in the database", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		session, jsonQueryBytes := newMongoDumpQuerySubtestFixture(t)
		md := newMongoDumpForBSONSubtest(t, "")

		require.NoError(
			t,
			os.WriteFile("example.json", jsonQueryBytes, 0777),
			"should write the query file",
		)
		md.InputOptions.QueryFile = "example.json"
		md.ToolOptions.DB = testDB
		md.OutputOptions.Out = "dump"
		dumpDir := testQuery(t, md, session)

		t.Cleanup(func() {
			assert.NoError(
				t,
				session.Database(testRestoreDB).Drop(context.Background()),
				"should drop the restore database",
			)
			assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
			assert.NoError(t, os.Remove("example.json"), "should remove the query file")
		})
	})

	t.Run("using mongodump against a collection that doesn't exist succeeds", func(t *testing.T) {
		setUpMongoDumpBSONSubtest(t)
		md, err := simpleMongoDumpInstance()
		require.NoError(t, err, "should build a mongodump instance")

		md.ToolOptions.DB = "nonExistentDB"
		md.ToolOptions.Collection = "nonExistentColl"

		require.NoError(t, md.Init(), "should initialize mongodump")
		require.NoError(t, md.Dump(), "should succeed dumping a nonexistent collection")
	})
}

// setUpMongoDumpBSONSubtest inserts fresh test data for one TestMongoDumpBSON
// subtest and registers its teardown, matching the setUp/Reset pair GoConvey
// ran around every leaf of the original nested test.
func setUpMongoDumpBSONSubtest(t *testing.T) {
	t.Helper()

	require.NoError(t, setUpMongoDumpTestData(t), "should set up test data")
	t.Cleanup(func() {
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})
}

// newMongoDumpForBSONSubtest builds the MongoDump instance shared by the
// WITHOUT-a-query subtests, with collName applied when it isn't empty.
func newMongoDumpForBSONSubtest(t *testing.T, collName string) *MongoDump {
	t.Helper()

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a mongodump instance")

	md.InputOptions.Query = ""
	if collName != "" {
		md.ToolOptions.Collection = collName
	}

	return md
}

// newMongoDumpQuerySubtestFixture builds the session and marshaled query
// shared by the --query and --queryFile subtests.
func newMongoDumpQuerySubtestFixture(t *testing.T) (*mongo.Client, []byte) {
	t.Helper()

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "should connect to the server")

	// expect 10 documents per collection
	bsonQuery := bson.M{"age": bson.M{"$lt": 10}}
	jsonQuery, err := bsonutil.ConvertBSONValueToLegacyExtJSON(bsonQuery)
	require.NoError(t, err, "should convert the query to extended json")
	jsonQueryBytes, err := json.Marshal(jsonQuery)
	require.NoError(t, err, "should marshal the query to json")

	return session, jsonQueryBytes
}

func TestMongoDumpBSONLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 4.4 or later; found %v", fcv)
	}

	log.SetWriter(io.Discard)

	require.NoError(t, setUpMongoDumpTestData(t), "should set up test data")
	t.Cleanup(func() {
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a mongodump instance")

	// testing that it dumps a collection with a name >238 bytes in the right format
	coll := session.Database(testDB).Collection(longCollectionName)
	_, err = coll.InsertOne(t.Context(), bson.M{"a": 1})
	require.NoError(t, err, "should insert a document into the long-named collection")
	//nolint:errcheck
	defer coll.Drop(t.Context())

	md.ToolOptions.Collection = longCollectionName
	require.NoError(t, md.Init(), "should initialize mongodump")

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	absDumpDir := filepath.FromSlash(filepath.Join(path, "dump_slash"))
	require.NoError(t, os.RemoveAll(absDumpDir), "should remove any pre-existing dump directory")
	require.False(t, fileDirExists(absDumpDir), "should not have a dump directory before dumping")

	dumpDBDir := filepath.FromSlash(filepath.Join("dump_slash", testDB))
	require.False(
		t,
		fileDirExists(dumpDBDir),
		"should not have a database directory before dumping",
	)

	md.OutputOptions.Out = "dump_slash"
	require.NoError(t, md.Dump(), "should dump the long-named collection")
	require.True(t, fileDirExists(dumpDBDir), "should create the database directory")

	// to a bson file
	oneBsonFile, err := os.Open(filepath.FromSlash(filepath.Join(dumpDBDir, longBsonName)))
	require.NoError(t, err, "should open the dumped bson file for the long collection name")
	oneBsonFile.Close()

	// to a metadata file
	oneMetaFile, err := os.Open(filepath.FromSlash(filepath.Join(dumpDBDir, longMetadataName)))
	require.NoError(t, err, "should open the dumped metadata file for the long collection name")
	oneMetaFile.Close()
}

func testPreludeMetadata(t *testing.T, md *MongoDump, dir string, serverVersion string) {
	t.Helper()

	require.False(t, fileDirExists(dir), "should not have a dump directory before dumping")
	require.NoError(t, md.Init(), "should initialize mongodump")
	require.NoError(t, md.Dump(), "should dump the database")

	preludeFilepath := filepath.Join(dir, "prelude.json")
	if md.OutputOptions.Gzip {
		preludeFilepath += ".gz"
	}
	require.True(t, fileDirExists(preludeFilepath), "should write a prelude file")

	var reader io.Reader
	preludeFile, err := os.Open(filepath.FromSlash(preludeFilepath))
	require.NoError(t, err, "should open the prelude file")
	reader = preludeFile
	defer preludeFile.Close()
	if md.OutputOptions.Gzip {
		zipfile, err := gzip.NewReader(preludeFile)
		require.NoError(t, err, "should open the gzipped prelude file")
		defer zipfile.Close()
		reader = zipfile
	}
	contents, err := io.ReadAll(reader)
	require.NoError(t, err, "should read the prelude file")

	var jsonResult map[string]any
	require.NoError(t, json.Unmarshal(contents, &jsonResult), "should unmarshal the prelude json")
	require.EqualValues(
		t,
		serverVersion,
		jsonResult["ServerVersion"],
		"should record the connected server version in the prelude",
	)
}

func TestDumpPreludeMetadataJson(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	t.Run(
		"writes prelude.json to the dump directory when dumping all databases",
		func(t *testing.T) {
			setUpPreludeMetadataSubtest(t)
			md, serverVersion := newMongoDumpForPreludeSubtest(t, "")

			dumpDir := preludeSubtestDumpDir(t, "dump")
			testPreludeMetadata(t, md, dumpDir, serverVersion)
		},
	)

	t.Run(
		"writes prelude.json.gz to the dump directory when dumping all databases with --gzip",
		func(t *testing.T) {
			setUpPreludeMetadataSubtest(t)
			md, serverVersion := newMongoDumpForPreludeSubtest(t, "")
			md.OutputOptions.Gzip = true

			dumpDir := preludeSubtestDumpDir(t, "dump")
			testPreludeMetadata(t, md, dumpDir, serverVersion)
		},
	)

	t.Run(
		"writes prelude.json to a user-specified output directory when dumping all databases",
		func(t *testing.T) {
			setUpPreludeMetadataSubtest(t)
			md, serverVersion := newMongoDumpForPreludeSubtest(t, "")
			md.OutputOptions.Out = "dump_output"

			dumpDir := preludeSubtestDumpDir(t, "dump_output")
			testPreludeMetadata(t, md, dumpDir, serverVersion)
		},
	)

	t.Run("writes prelude.json to the dump directory when dumping one db", func(t *testing.T) {
		setUpPreludeMetadataSubtest(t)
		md, serverVersion := newMongoDumpForPreludeSubtest(t, testDB)

		dumpDir := preludeSubtestDumpDir(t, "dump")
		dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
		testPreludeMetadata(t, md, dumpDBDir, serverVersion)
	})

	t.Run(
		"does not fail and does not create prelude.json when the dump directory is not created",
		func(t *testing.T) {
			setUpPreludeMetadataSubtest(t)
			md, _ := newMongoDumpForPreludeSubtest(t, "nonExistentDB")

			path, err := os.Getwd()
			require.NoError(t, err, "should get the working directory")
			dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
			dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, "nottestdb"))
			t.Cleanup(func() {
				assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
			})

			require.NoError(t, md.Init(), "should initialize mongodump")
			require.NoError(t, md.Dump(), "should succeed dumping a nonexistent database")

			assert.False(t, fileDirExists(dumpDir), "should not create the dump directory")
			assert.False(t, fileDirExists(dumpDBDir), "should not create the database directory")
			assert.False(
				t,
				fileDirExists(filepath.Join(dumpDir, "prelude.json")),
				"should not create a prelude file in the dump directory",
			)
			assert.False(
				t,
				fileDirExists(filepath.Join(dumpDBDir, "prelude.json")),
				"should not create a prelude file in the database directory",
			)
		},
	)
}

// setUpPreludeMetadataSubtest inserts fresh test data for one
// TestDumpPreludeMetadataJson subtest and registers its teardown.
func setUpPreludeMetadataSubtest(t *testing.T) {
	t.Helper()

	require.NoError(t, setUpMongoDumpTestData(t), "should set up test data")
	t.Cleanup(func() {
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})
}

// newMongoDumpForPreludeSubtest builds a MongoDump instance and reports the
// connected server version. dbName, when non-empty, restricts the dump to
// that database; empty clears both the database and collection so the whole
// server gets dumped.
func newMongoDumpForPreludeSubtest(t *testing.T, dbName string) (*MongoDump, string) {
	t.Helper()

	sessionProvider, _, _ := testutil.GetBareSessionProvider()
	require.NotNil(t, sessionProvider, "should get a session provider")
	serverVersion, err := sessionProvider.ServerVersion()
	require.NoError(t, err, "should get the server version")

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a mongodump instance")

	if dbName != "" {
		md.ToolOptions.DB = dbName
	} else {
		md.ToolOptions.DB = ""
		md.ToolOptions.Collection = ""
	}

	return md, serverVersion
}

// preludeSubtestDumpDir removes any pre-existing dump directory named name
// under the working directory and returns its absolute path, matching the
// per-subtest cleanup GoConvey's Reset performed for each dump location.
func preludeSubtestDumpDir(t *testing.T, name string) string {
	t.Helper()

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	dumpDir := filepath.FromSlash(filepath.Join(path, name))
	require.NoError(t, os.RemoveAll(dumpDir), "should remove any pre-existing dump directory")

	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
	})

	return dumpDir
}

func TestMongoDumpMetaData(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	session, err := testutil.GetBareSession()
	require.NotNil(t, session, "should get a session")
	require.NoError(t, err, "should connect to the server")

	require.NoError(t, setUpMongoDumpTestData(t), "should set up test data")
	t.Cleanup(func() {
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})

	// testing that the dumped directory contains information about indexes
	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a mongodump instance")

	md.OutputOptions.Out = "dump"
	require.NoError(t, md.Init(), "should initialize mongodump")
	require.NoError(t, md.Dump(), "should dump the database")

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")
	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
	dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
	require.True(t, fileDirExists(dumpDir), "should create the dump directory")
	require.True(t, fileDirExists(dumpDBDir), "should create the database directory")
	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
	})

	// having one metadata file per collection
	c1, err := countNonIndexBSONFiles(dumpDBDir)
	require.NoError(t, err, "should count the dumped bson files")

	c2, err := countMetaDataFiles(dumpDBDir)
	require.NoError(t, err, "should count the dumped metadata files")

	require.EqualValues(t, c2, c1, "should write one metadata file per collection")

	// and that the JSON in a metadata file is valid
	metaFiles, err := getMatchingFiles(dumpDBDir, ".*\\.metadata\\.json")
	require.NoError(t, err, "should list the metadata files")
	require.Greater(t, len(metaFiles), 0, "should find at least one metadata file")

	oneMetaFile, err := os.Open(filepath.FromSlash(filepath.Join(dumpDBDir, metaFiles[0])))
	require.NoError(t, err, "should open a metadata file")
	defer oneMetaFile.Close()

	contents, err := io.ReadAll(oneMetaFile)
	require.NoError(t, err, "should read the metadata file")
	var jsonResult map[string]any
	require.NoError(t, json.Unmarshal(contents, &jsonResult), "should unmarshal the metadata json")

	// and contains an 'indexes' key
	_, ok := jsonResult["indexes"]
	assert.True(t, ok, "should include an indexes key in the metadata")

	// and contains a 'collectionName' key
	_, ok = jsonResult["collectionName"]
	assert.True(t, ok, "should include a collectionName key in the metadata")

	fcv := testutil.GetFCV(session)
	cmp, err := testutil.CompareFCV(fcv, "3.6")
	require.NoError(t, err, "should compare the server's FCV")
	if cmp >= 0 {
		// and on FCV 3.6+, contains a 'uuid' key
		uuid, ok := jsonResult["uuid"]
		require.True(t, ok, "should include a uuid key in the metadata on FCV 3.6+")
		checkUUID := regexp.MustCompile(`(?i)^[a-z0-9]{32}$`)

		uuidStr, ok := uuid.(string)
		require.True(t, ok, "should represent the uuid as a string")

		assert.True(
			t,
			checkUUID.MatchString(uuidStr),
			"should format the uuid as 32 hex characters",
		)
		// XXX useless -- xdg, 2018-09-21
		assert.NoError(t, err, "should compare the server's FCV")
	}
}

func TestMongoDumpOplog(t *testing.T) {
	// Disabled: see TOOLS-2657
	t.Skip()

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}
	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available: %v", err)
	}
	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.SkipNow()
	}
	log.SetWriter(io.Discard)

	// Start with clean filesystem
	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
	dumpOplogFile := filepath.FromSlash(filepath.Join(dumpDir, "oplog.bson"))

	err = os.RemoveAll(dumpDir)
	require.NoError(t, err, "should remove any existing dump directory")
	require.False(t, fileDirExists(dumpDir), "should have no dump directory before running")

	// Start with clean database
	require.NoError(t, tearDownMongoDumpTestData(t), "should tear down existing test data")

	// Prepare mongodump with options
	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.OutputOptions.Oplog = true
	md.ToolOptions.Namespace = &options.Namespace{}
	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")

	// Start inserting docs in the background so the oplog has data
	ready := make(chan struct{})
	done := make(chan struct{})
	errs := make(chan error, 1)
	go backgroundInsert(t, ready, done, errs)
	<-ready

	// Run mongodump
	err = md.Dump()
	require.NoError(t, err, "should dump with the oplog option")

	// Stop background insertion
	close(done)
	err = <-errs
	require.NoError(t, err, "should insert documents in the background without error")

	// Check for and read the oplog file
	require.True(t, fileDirExists(dumpDir), "should create the dump directory")
	require.True(t, fileDirExists(dumpOplogFile), "should create the oplog file")

	oplogFile, err := os.Open(dumpOplogFile)
	defer oplogFile.Close()
	require.NoError(t, err, "should open the oplog file")

	rdr := db.NewBSONSource(oplogFile)
	iter := db.NewDecodedBSONSource(rdr)

	fcv := testutil.GetFCV(session)
	cmp, err := testutil.CompareFCV(fcv, "3.6")
	require.NoError(t, err, "should compare the server's FCV")

	withUI := countOplogUI(iter)
	require.NoError(t, iter.Err(), "should decode every oplog entry")

	if cmp >= 0 {
		// for FCV 3.6+, should have 'ui' field in oplog entries
		assert.Greater(t, withUI, 0, "should include a ui field in oplog entries on FCV 3.6+")
	} else {
		// for FCV <3.6, should no have 'ui' field in oplog entries
		assert.Equal(t, 0, withUI, "should have no ui field in oplog entries below FCV 3.6")
	}

	// Cleanup
	require.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
	require.NoError(t, tearDownMongoDumpTestData(t), "should tear down test data")
}

// Test dumping a collection with autoIndexId:false.  As of MongoDB 4.0,
// this is only allowed on the 'local' database.
func TestMongoDumpTOOLS2174(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}

	serverVersion, err := sessionProvider.ServerVersionArray()
	if err != nil {
		t.Fatalf("Could not get Server version: %v", err)
	}
	if serverVersion.GTE(db.Version{8, 2, 0}) {
		t.Skipf(
			"createCollection no longer accepts autoIndexID as of Server version 8.2.0; testing with %s",
			serverVersion.String(),
		)
	}

	collName := "tools-2174"
	dbName := "local"

	var r1 bson.M
	err = sessionProvider.Run(bson.D{{"drop", collName}}, &r1, dbName)
	if err != nil {
		var commandErr mongo.CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != 26 {
			t.Fatalf("Failed to run drop: %v", err)
		}
	}

	createCmd := bson.D{
		{"create", collName},
		{"autoIndexId", false},
	}
	var r2 bson.M
	err = sessionProvider.Run(createCmd, &r2, dbName)
	if err != nil {
		t.Fatalf("Error creating capped, no-autoIndexId collection: %v", err)
	}

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.ToolOptions.Collection = collName
	md.ToolOptions.DB = dbName
	md.OutputOptions.Out = "dump"
	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")
	err = md.Dump()
	require.NoError(t, err, "should dump a capped, autoIndexId:false collection")
}

// Test dumping a collection while respecting no index scan for wired tiger.
func TestMongoDumpTOOLS1952(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	collName := "tools-1952-dump"
	dbName := "test"
	ns := dbName + "." + collName

	var r1 bson.M

	dbStruct := session.Database(dbName)

	err = sessionProvider.Run(bson.D{{"drop", collName}}, &r1, dbName)
	if err != nil {
		var commandErr mongo.CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != 26 {
			t.Fatalf("Failed to run drop: %v", err)
		}
	}

	createCmd := bson.D{
		{"create", collName},
	}
	var r2 bson.M
	err = sessionProvider.Run(createCmd, &r2, dbName)
	if err != nil {
		t.Fatalf("Error creating collection: %v", err)
	}

	// Turn on profiling.
	if err = turnOnProfiling(dbName); err != nil {
		t.Fatalf("Failed to turn on profiling: %v", err)
	}

	profileCollection := dbStruct.Collection("system.profile")

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.ToolOptions.Collection = collName
	md.ToolOptions.DB = dbName
	md.OutputOptions.Out = "dump"
	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")
	err = md.Dump()
	require.NoError(t, err, "should dump the collection")

	count, err := countSnapshotCmds(t, profileCollection, ns)
	require.NoError(t, err, "should count snapshot commands in the profile collection")

	// On modern storage engines, there should be no query that matches.
	require.Zero(t, count, "should perform no snapshot query on modern storage engines")
}

// Test the fix for nil pointer bug when getCollectionInfo failed.
func TestMongoDumpTOOLS2498(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}

	collName := "tools-2498-dump"
	dbName := "test"

	var r1 bson.M
	err = sessionProvider.Run(bson.D{{"drop", collName}}, &r1, dbName)
	if err != nil {
		var commandErr mongo.CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != 26 {
			t.Fatalf("Failed to run drop: %v", err)
		}
	}

	createCmd := bson.D{
		{"create", collName},
	}
	var r2 bson.M
	err = sessionProvider.Run(createCmd, &r2, dbName)
	if err != nil {
		t.Fatalf("Error creating collection: %v", err)
	}

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.ToolOptions.Collection = collName
	md.ToolOptions.DB = dbName
	md.OutputOptions.Out = "dump"
	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")

	require.NoError(t, failpoint.DefaultManager.Parse(failpoint.PauseUntilResumed.String()))
	defer failpoint.DefaultManager.Reset()

	dumpErrCh := make(chan error, 1)
	go func() { dumpErrCh <- md.Dump() }()

	fp, ok := failpoint.DefaultManager.Get(failpoint.PauseUntilResumed)
	require.True(t, ok, "should find the pause-until-resumed failpoint")
	require.NoError(t, fp.Reached(context.TODO()))
	session, _ := md.SessionProvider.GetSession()
	disconnectErr := session.Disconnect(t.Context())
	require.NoError(t, disconnectErr, "should disconnect the session")
	fp.Signal()

	err = <-dumpErrCh
	// Mongodump should not panic, but return correct the error if getCollectionInfo failed.
	require.Error(t, err, "should return an error rather than panic when getCollectionInfo fails")
	require.Contains(
		t,
		err.Error(),
		"client is disconnected",
		"should report the disconnected client as the cause",
	)
}

func TestMongoDumpOrderedQuery(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	err := setUpMongoDumpTestData(t)
	require.NoError(t, err, "should set up test data")
	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")
	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))

	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})

	// If order is not preserved, probabilistically, some of these
	// loops will fail.
	for i := 0; i < 100; i++ {
		require.NoError(
			t,
			os.RemoveAll(dumpDir),
			"should remove the dump directory before each run",
		)

		md, err := simpleMongoDumpInstance()
		require.NoError(t, err, "should build a MongoDump instance")

		md.InputOptions.Query = `{"coords":{"x":0,"y":1}}`
		md.ToolOptions.Collection = testCollectionNames[0]
		md.ToolOptions.DB = testDB
		md.OutputOptions.Out = "dump"
		err = md.Init()
		require.NoError(t, err, "should initialize the MongoDump instance")
		err = md.Dump()
		require.NoError(t, err, "should dump with the ordered query")

		dumpBSON := filepath.FromSlash(
			filepath.Join(dumpDir, testDB, testCollectionNames[0]+".bson"),
		)

		file, err := os.Open(dumpBSON)
		require.NoError(t, err, "should open the dumped BSON file")

		bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(file))

		var count int
		var result bson.M
		for bsonSource.Next(&result) {
			count++
		}
		require.NoError(t, bsonSource.Err(), "should decode every document in the dump")

		require.Equal(t, 1, count, "should match exactly one document per ordered query")

		bsonSource.Close()
		file.Close()
	}
}

func TestMongoDumpViewsAsCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	t.Run("having one metadata file per read-only view", func(t *testing.T) {
		dumpDBDir, _, _ := setUpViewsAsCollectionsDump(t, "dump_view_as_collection")

		c1, err := countNonIndexBSONFiles(dumpDBDir)
		require.NoError(t, err, "should count non-index BSON files")

		c2, err := countMetaDataFiles(dumpDBDir)
		require.NoError(t, err, "should count metadata files")

		assert.Equal(t, c2, c1, "should write one metadata file per read-only view")
	})

	t.Run("testing dumping a view, we should not hint index", func(t *testing.T) {
		_, dbName, colName := setUpViewsAsCollectionsDump(t, "dump_view_as_collection")

		session, err := testutil.GetBareSession()
		require.NoError(t, err, "should connect to the server")

		dbStruct := session.Database(dbName)
		profileCollection := dbStruct.Collection("system.profile")
		ns := dbName + "." + colName
		count, err := countSnapshotCmds(t, profileCollection, ns)
		require.NoError(t, err, "should count snapshot commands in the profile collection")

		// view dump should not do collection scan
		assert.Zero(t, count, "should perform no collection scan when dumping a view")
	})
}

// setUpViewsAsCollectionsDump sets up test data and a view, dumps the
// database with ViewsAsCollections enabled, and registers its own teardown
// so each subtest gets a fresh dump. Returns the dumped database directory,
// the database name, and colName unchanged for the caller's convenience.
func setUpViewsAsCollectionsDump(t *testing.T, colName string) (string, string, string) {
	t.Helper()

	err := setUpMongoDumpTestData(t)
	require.NoError(t, err, "should set up test data")

	dbName := testDB
	err = setUpDBView(dbName, colName)
	require.NoError(t, err, "should create the view")

	err = turnOnProfiling(testDB)
	require.NoError(t, err, "should turn on profiling")

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.ToolOptions.DB = testDB
	md.OutputOptions.Out = "dump"
	md.OutputOptions.ViewsAsCollections = true

	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")

	err = md.Dump()
	require.NoError(t, err, "should dump the database with views as collections")

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
	dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
	require.True(t, fileDirExists(dumpDir), "should create the dump directory")
	require.True(t, fileDirExists(dumpDBDir), "should create the database dump directory")

	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})

	return dumpDBDir, dbName, colName
}

func TestMongoDumpViews(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	t.Run("having one metadata file per view", func(t *testing.T) {
		dumpDBDir, _, _ := setUpViewsDump(t, "dump_views")

		c1, err := countMetaDataFiles(dumpDBDir)
		require.NoError(t, err, "should count metadata files")

		assert.Greater(t, c1, 0, "should write at least one metadata file per view")
	})

	t.Run("testing dumping a view, we should not hint index", func(t *testing.T) {
		_, dbName, colName := setUpViewsDump(t, "dump_views")

		session, err := testutil.GetBareSession()
		require.NoError(t, err, "should connect to the server")

		dbStruct := session.Database(dbName)
		profileCollection := dbStruct.Collection("system.profile")
		ns := dbName + "." + colName
		count, err := countSnapshotCmds(t, profileCollection, ns)
		require.NoError(t, err, "should count snapshot commands in the profile collection")

		// view dump should not do collection scan
		assert.Zero(t, count, "should perform no collection scan when dumping a view")
	})
}

// setUpViewsDump sets up test data and a view, dumps the database, and
// registers its own teardown so each subtest gets a fresh dump. Returns the
// dumped database directory, the database name, and colName unchanged for
// the caller's convenience.
func setUpViewsDump(t *testing.T, colName string) (string, string, string) {
	t.Helper()

	err := setUpMongoDumpTestData(t)
	require.NoError(t, err, "should set up test data")

	dbName := testDB
	err = setUpDBView(dbName, colName)
	require.NoError(t, err, "should create the view")

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.ToolOptions.DB = testDB
	md.OutputOptions.Out = "dump"

	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")

	err = md.Dump()
	require.NoError(t, err, "should dump the database")

	path, err := os.Getwd()
	require.NoError(t, err, "should get the working directory")

	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
	dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
	require.True(t, fileDirExists(dumpDir), "should create the dump directory")
	require.True(t, fileDirExists(dumpDBDir), "should create the database dump directory")

	t.Cleanup(func() {
		assert.NoError(t, os.RemoveAll(dumpDir), "should remove the dump directory")
		assert.NoError(t, tearDownMongoDumpTestDataInCleanup(), "should tear down test data")
	})

	return dumpDBDir, dbName, colName
}

func TestMongoDumpCollectionOutputPath(t *testing.T) {
	t.Skip("disabled: see TOOLS-2658")

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
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

func TestCount(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	err := setUpMongoDumpTestData(t)
	require.NoError(t, err, "should set up test data")

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "should connect to the server")

	collection := session.Database(testDB).Collection(testCollectionNames[0])
	restoredDB := session.Database(testDB)
	//nolint:errcheck
	defer restoredDB.Drop(t.Context())

	t.Run("count collection without filter", func(t *testing.T) {
		findQuery := &db.DeferredQuery{Coll: collection}
		cnt, err := findQuery.Count(false)
		require.NoError(t, err, "should count documents with no filter")
		assert.Equal(t, 10, cnt, "should count every document")

		findQuery = &db.DeferredQuery{Coll: collection, Filter: bson.M{}}
		cnt, err = findQuery.Count(false)
		require.NoError(t, err, "should count documents with an empty bson.M filter")
		assert.Equal(t, 10, cnt, "should count every document")

		findQuery = &db.DeferredQuery{Coll: collection, Filter: bson.D{}}
		cnt, err = findQuery.Count(false)
		require.NoError(t, err, "should count documents with an empty bson.D filter")
		assert.Equal(t, 10, cnt, "should count every document")
	})

	t.Run("count collection with filter in BSON.M", func(t *testing.T) {
		findQuery := &db.DeferredQuery{Coll: collection, Filter: bson.M{"age": 1}}
		cnt, err := findQuery.Count(false)
		require.NoError(t, err, "should count documents matching a bson.M filter")
		assert.Equal(t, 1, cnt, "should count only matching documents")
	})

	t.Run("count collection with filter in BSON.D", func(t *testing.T) {
		findQuery := &db.DeferredQuery{Coll: collection, Filter: bson.D{{"age", 1}}}
		cnt, err := findQuery.Count(false)
		require.NoError(t, err, "should count documents matching a bson.D filter")
		assert.Equal(t, 1, cnt, "should count only matching documents")
	})
}

func TestTimeseriesCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "get session")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 5.0 or later; found %v", fcv)
	}

	sp, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	serverVersion, err := sp.ServerVersionArray()
	require.NoError(t, err, "get server version")

	colName := "timeseriesColl"
	dbName := "timeseries_test_DB"
	testutil.SetUpTimeseries(t, dbName, colName)

	defer func() {
		err := dropDB(t, dbName)
		require.NoError(t, err, "drop timeseries coll")
	}()

	setup := func(kind string) *MongoDump {
		md, err := simpleMongoDumpInstance()
		require.NoError(t, err)

		md.ToolOptions.DB = dbName

		switch kind {
		case "archive":
			md.OutputOptions.Out = ""
			md.OutputOptions.Archive = "dump.archive"
		case "dir":
			md.OutputOptions.Out = "dump"
		default:
			panic("unknown setup kind " + kind)
		}

		return md
	}

	t.Run("dump to archive", func(t *testing.T) {
		runArchiveTest := func(t *testing.T, md *MongoDump) {
			err = md.Init()
			require.NoError(t, err)

			err = md.Dump()
			require.NoError(t, err)

			path, err := os.Getwd()
			require.NoError(t, err)

			archiveFilePath := filepath.FromSlash(filepath.Join(path, "dump.archive"))

			archiveFile, err := os.Open(archiveFilePath)
			require.NoError(t, err)

			archiveReader := &archive.Reader{
				In:      archiveFile,
				Prelude: &archive.Prelude{},
			}

			err = archiveReader.Prelude.Read(archiveReader.In)
			require.NoError(t, err)

			collectionMetadatas, ok := archiveReader.Prelude.NamespaceMetadatasByDB[dbName]
			assert.True(t, ok)

			require.Len(t, collectionMetadatas, 1)
			assert.Equal(t, colName, collectionMetadatas[0].Collection)

			pe, err := archiveReader.Prelude.NewPreludeExplorer()
			require.NoError(t, err)

			archiveContents, err := pe.ReadDir()
			require.NoError(t, err)

			expectedCollFile := timeseriesCollName(serverVersion, colName) + ".bson"

			for _, dirlike := range archiveContents {
				if !dirlike.IsDir() || dirlike.Name() != dbName {
					continue
				}

				dbContents, err := dirlike.ReadDir()
				require.NoError(t, err)

				assert.Len(t, dbContents, 2)

				for _, file := range dbContents {
					assert.Contains(
						t,
						[]string{
							colName + ".metadata.json",
							expectedCollFile,
						},
						file.Name(),
					)
				}
			}

			require.NoError(t, archiveFile.Close())
			require.NoError(t, os.RemoveAll(archiveFilePath))
		}

		t.Run("whole db", func(t *testing.T) {
			md := setup("archive")
			runArchiveTest(t, md)
		})

		t.Run("specified collection", func(t *testing.T) {
			md := setup("archive")
			md.ToolOptions.DB = dbName
			md.ToolOptions.Collection = colName
			runArchiveTest(t, md)
		})

		t.Run("exclude system.buckets.coll", func(t *testing.T) {
			md := setup("archive")
			md.OutputOptions.ExcludedCollections = []string{common.TimeseriesBucketPrefix + colName}
			runArchiveTest(t, md)
		})

		t.Run("exclude system.buckets prefix", func(t *testing.T) {
			md := setup("archive")
			md.OutputOptions.ExcludedCollectionPrefixes = []string{common.TimeseriesBucketPrefix}
			runArchiveTest(t, md)
		})
	})

	t.Run("dump to directory", func(t *testing.T) {
		runDirTest := func(t *testing.T, md *MongoDump) {
			err = md.Init()
			require.NoError(t, err)

			err = md.Dump()
			require.NoError(t, err)

			path, err := os.Getwd()
			require.NoError(t, err)

			dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
			dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, dbName))
			metadataFile := filepath.FromSlash(
				filepath.Join(dumpDBDir, colName+".metadata.json"),
			)

			expectedCollFile := timeseriesCollName(serverVersion, colName)
			bsonFile := filepath.FromSlash(
				filepath.Join(dumpDBDir, expectedCollFile+".bson"),
			)
			assert.True(t, fileDirExists(dumpDir))
			assert.True(t, fileDirExists(dumpDBDir))
			assert.True(t, fileDirExists(metadataFile))
			assert.True(t, fileDirExists(bsonFile), bsonFile)

			allFiles, err := getMatchingFiles(dumpDBDir, ".*"+colName+".*")
			require.NoError(t, err)
			assert.Len(t, allFiles, 2)

			info, err := os.Stat(bsonFile)
			require.NoError(t, err)
			assert.NotZero(t, info.Size())

			require.NoError(t, os.RemoveAll(dumpDir))
		}

		t.Run("whole db", func(t *testing.T) {
			md := setup("dir")
			runDirTest(t, md)
		})

		t.Run("specified collection", func(t *testing.T) {
			md := setup("dir")
			md.ToolOptions.DB = dbName
			md.ToolOptions.Collection = colName
			runDirTest(t, md)
		})

		t.Run("exclude system.buckets.coll", func(t *testing.T) {
			md := setup("dir")
			md.OutputOptions.ExcludedCollections = []string{common.TimeseriesBucketPrefix + colName}
			runDirTest(t, md)
		})

		t.Run("exclude system.buckets prefix", func(t *testing.T) {
			md := setup("dir")
			md.OutputOptions.ExcludedCollectionPrefixes = []string{common.TimeseriesBucketPrefix}
			runDirTest(t, md)
		})
	})

	t.Run("exclude to archive", func(t *testing.T) {
		runArchiveTest := func(t *testing.T, md *MongoDump) {
			err = md.Init()
			require.NoError(t, err)

			err = md.Dump()
			require.NoError(t, err)

			path, err := os.Getwd()
			require.NoError(t, err)

			archiveFilePath := filepath.FromSlash(filepath.Join(path, "dump.archive"))

			archiveFile, err := os.Open(archiveFilePath)
			require.NoError(t, err)
			archiveReader := &archive.Reader{
				In:      archiveFile,
				Prelude: &archive.Prelude{},
			}

			err = archiveReader.Prelude.Read(archiveReader.In)
			require.NoError(t, err)

			_, ok := archiveReader.Prelude.NamespaceMetadatasByDB[dbName]
			assert.False(t, ok)

			require.NoError(t, archiveFile.Close())
			require.NoError(t, os.RemoveAll(archiveFilePath))
		}

		t.Run("explicit exclude", func(t *testing.T) {
			md := setup("archive")
			md.OutputOptions.ExcludedCollections = []string{colName}
			runArchiveTest(t, md)
		})

		t.Run("exclude by prefix", func(t *testing.T) {
			md := setup("archive")
			md.OutputOptions.ExcludedCollectionPrefixes = []string{colName[:5]}
			runArchiveTest(t, md)
		})
	})

	t.Run("exclude to directory", func(t *testing.T) {
		runDirTest := func(t *testing.T, md *MongoDump) {
			err = md.Init()
			require.NoError(t, err)

			err = md.Dump()
			require.NoError(t, err)

			path, err := os.Getwd()
			require.NoError(t, err)

			dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
			assert.False(t, fileDirExists(dumpDir))

			require.NoError(t, os.RemoveAll(dumpDir))
		}

		t.Run("explicit exclude", func(t *testing.T) {
			md := setup("dir")
			md.OutputOptions.ExcludedCollections = []string{colName}
			runDirTest(t, md)
		})

		t.Run("exclude by prefix", func(t *testing.T) {
			md := setup("dir")
			md.OutputOptions.ExcludedCollectionPrefixes = []string{colName[:5]}
			runDirTest(t, md)
		})
	})

	t.Run("specify buckets collection explicitly", func(t *testing.T) {
		md := setup("dir")
		md.ToolOptions.DB = dbName
		md.ToolOptions.Collection = common.TimeseriesBucketPrefix + colName

		err = md.Init()
		require.Error(t, err)
		assert.ErrorContains(
			t,
			err,
			"cannot specify a system.buckets collection in --collection. "+
				"Specifying the timeseries collection will dump the system.buckets collection",
		)
	})

	t.Run("query by metadata field", func(t *testing.T) {
		runQueryTest := func(t *testing.T, md *MongoDump) {
			err = md.Init()
			require.NoError(t, err)

			err = md.Dump()
			require.NoError(t, err)

			path, err := os.Getwd()
			require.NoError(t, err)

			dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
			dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, dbName))
			metadataFile := filepath.FromSlash(
				filepath.Join(dumpDBDir, colName+".metadata.json"),
			)
			bsonFile := filepath.FromSlash(
				filepath.Join(dumpDBDir, timeseriesCollName(serverVersion, colName)+".bson"),
			)
			assert.True(t, fileDirExists(dumpDir))
			assert.True(t, fileDirExists(dumpDBDir))
			assert.True(t, fileDirExists(metadataFile))
			assert.True(t, fileDirExists(bsonFile))

			allFiles, err := getMatchingFiles(dumpDBDir, ".*"+colName+".*")
			require.NoError(t, err)
			assert.Len(t, allFiles, 2)

			info, err := os.Stat(bsonFile)
			require.NoError(t, err)
			assert.NotZero(t, info.Size())

			fd, err := os.Open(bsonFile)
			require.NoError(t, err)

			bsonSource := db.NewBSONSource(fd)

			matchedDoc := bson.Raw(bsonSource.LoadNext())
			rawVal, err := matchedDoc.LookupErr("meta", "device")
			require.NoError(t, err)

			val, ok := rawVal.Int32OK()
			require.True(t, ok)
			assert.EqualValues(t, 1, val)

			// only one bucket document should be matched
			require.Nil(t, bsonSource.LoadNext())

			require.NoError(t, fd.Close())
			require.NoError(t, os.RemoveAll(dumpDir))
			os.Remove("ts_query.json")
		}

		t.Run("query", func(t *testing.T) {
			md := setup("dir")
			md.ToolOptions.Collection = colName
			md.InputOptions.Query = "{\"my_meta.device\": 1}"
			runQueryTest(t, md)
		})

		t.Run("queryFile", func(t *testing.T) {
			md := setup("dir")
			md.ToolOptions.Collection = colName

			err = os.WriteFile("ts_query.json", []byte("{\"my_meta.device\": 1}"), 0777)
			require.NoError(t, err)

			md.InputOptions.QueryFile = "ts_query.json"
			runQueryTest(t, md)
		})
	})

	t.Run("query by non-metadata field", func(t *testing.T) {
		runQueryTest := func(t *testing.T, md *MongoDump) {
			err = md.Init()
			require.NoError(t, err)

			err = md.Dump()
			require.Error(t, err)
			assert.ErrorContains(
				t,
				err,
				"mongodump only processes queries on metadata fields for timeseries collections",
			)

			os.Remove("ts_query.json")
		}

		t.Run("query", func(t *testing.T) {
			md := setup("dir")
			md.ToolOptions.Collection = colName
			md.InputOptions.Query = "{\"wrong.device\": 1}"
			runQueryTest(t, md)
		})

		t.Run("queryFile", func(t *testing.T) {
			md := setup("dir")
			md.ToolOptions.Collection = colName

			err = os.WriteFile("ts_query.json", []byte("{\"wrong.device\": 1}"), 0777)
			require.NoError(t, err)

			md.InputOptions.QueryFile = "ts_query.json"
			runQueryTest(t, md)
		})
	})

}

func TestDumpTimeseriesCollectionsWithMixedSchema(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "get session")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 5.0 or later; found %v", fcv)
	}

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "get server version from session provider")

	colName := "timeseries_mixed_schema"
	dbName := "timeseries_test_DB"

	setupTimeseriesWithMixedSchema(t, dbName, colName)

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	md.ToolOptions.DB = dbName
	md.OutputOptions.Out = "dump"
	md.OutputOptions.Out = ""
	md.OutputOptions.Archive = "dump.archive"

	require.NoError(t, md.Init())

	require.NoError(t, md.Dump())

	path, err := os.Getwd()
	require.NoError(t, err)

	archiveFilePath := filepath.FromSlash(filepath.Join(path, "dump.archive"))

	archiveFile, err := os.Open(archiveFilePath)
	require.NoError(t, err)
	archiveReader := &archive.Reader{
		In:      archiveFile,
		Prelude: &archive.Prelude{},
	}

	require.NoError(t, archiveReader.Prelude.Read(archiveReader.In))

	collectionMetadatas, ok := archiveReader.Prelude.NamespaceMetadatasByDB[dbName]
	require.True(t, ok)

	require.Len(t, collectionMetadatas, 1)
	require.Equal(t, colName, collectionMetadatas[0].Collection)

	pe, err := archiveReader.Prelude.NewPreludeExplorer()
	require.NoError(t, err)

	archiveContents, err := pe.ReadDir()
	require.NoError(t, err)

	expectedCollFile := timeseriesCollName(serverVersion, colName) + ".bson"

	for _, dirlike := range archiveContents {
		if dirlike.IsDir() && dirlike.Name() == dbName {
			dbContents, err := dirlike.ReadDir()
			require.NoError(t, err)

			require.Len(t, dbContents, 2)

			for _, file := range dbContents {
				require.Contains(
					t,
					[]string{colName + ".metadata.json", expectedCollFile},
					file.Name(),
				)
			}
		}
	}

	require.NoError(t, archiveFile.Close())
	require.NoError(t, os.Remove(archiveFilePath))
	require.NoError(t, session.Database(dbName).Collection(colName).Drop(t.Context()))
}

func TestFailDuringResharding(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	testutil.SkipForAtlasCluster(
		t,
		"this test requires permissions that may not be available for an Atlas cluster",
	)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must get a session provider")

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "must get a session")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.9"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 4.9 or later; found %v", fcv)
	}

	ctx := t.Context()

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.Skipf("Not for replica sets")
	}

	defaultErrorMsg := "detected resharding in progress. Cannot dump with --oplog while resharding"
	oplogErrorMsg := "cannot dump with oplog while resharding"

	t.Run("dump should fail if config.reshardingOperations exists on source", func(t *testing.T) {
		md := newFailDuringReshardingMongoDump(t)
		err = session.Database("config").CreateCollection(ctx, "reshardingOperations")
		require.NoError(t, err, "should create the reshardingOperations collection")
		//nolint:errcheck
		defer session.Database("config").Collection("reshardingOperations").Drop(ctx)

		err = md.Dump()
		require.Error(t, err, "should refuse to dump while resharding is in progress")
		assert.Contains(
			t,
			err.Error(),
			defaultErrorMsg,
			"should explain that resharding is in progress",
		)
	})

	t.Run(
		"dump should fail if config.localReshardingOperations.donor exists on source",
		func(t *testing.T) {
			md := newFailDuringReshardingMongoDump(t)
			err = session.Database("config").
				CreateCollection(ctx, "localReshardingOperations.donor")
			require.NoError(t, err, "should create the localReshardingOperations.donor collection")
			//nolint:errcheck
			defer session.Database("config").
				Collection("localReshardingOperations.donor").
				Drop(ctx)

			err = md.Dump()
			require.Error(t, err, "should refuse to dump while resharding is in progress")
			assert.Contains(
				t,
				err.Error(),
				defaultErrorMsg,
				"should explain that resharding is in progress",
			)
		},
	)

	t.Run(
		"dump should fail if config.localReshardingOperations.recipient exists on source",
		func(t *testing.T) {
			md := newFailDuringReshardingMongoDump(t)
			err = session.Database("config").
				CreateCollection(ctx, "localReshardingOperations.recipient")
			require.NoError(
				t,
				err,
				"should create the localReshardingOperations.recipient collection",
			)
			//nolint:errcheck
			defer session.Database("config").
				Collection("localReshardingOperations.recipient").
				Drop(ctx)

			err = md.Dump()
			require.Error(t, err, "should refuse to dump while resharding is in progress")
			assert.Contains(
				t,
				err.Error(),
				defaultErrorMsg,
				"should explain that resharding is in progress",
			)
		},
	)

	t.Run("dump should fail if config.reshardingOperations created in oplog", func(t *testing.T) {
		md := newFailDuringReshardingMongoDump(t)
		require.NoError(t, failpoint.DefaultManager.Parse(failpoint.PauseUntilResumed.String()))
		defer failpoint.DefaultManager.Reset()

		dumpErrCh := make(chan error, 1)
		go func() { dumpErrCh <- md.Dump() }()

		fp, ok := failpoint.DefaultManager.Get(failpoint.PauseUntilResumed)
		require.True(t, ok, "should find the PauseUntilResumed failpoint")
		require.NoError(t, fp.Reached(context.TODO()))
		sessErr1 := session.Database("config").CreateCollection(ctx, "reshardingOperations")
		sessErr2 := session.Database("config").Collection("reshardingOperations").Drop(ctx)
		fp.Signal()

		err = <-dumpErrCh

		require.Error(
			t,
			err,
			"should refuse to dump while resharding is created during the oplog dump",
		)
		assert.Contains(
			t,
			err.Error(),
			oplogErrorMsg,
			"should explain that resharding is in progress",
		)
		assert.NoError(t, sessErr1, "should create the reshardingOperations collection")
		assert.NoError(t, sessErr2, "should drop the reshardingOperations collection")
	})

	t.Run(
		"dump should fail if config.localReshardingOperations.donor created in oplog",
		func(t *testing.T) {
			md := newFailDuringReshardingMongoDump(t)
			require.NoError(
				t,
				failpoint.DefaultManager.Parse(failpoint.PauseUntilResumed.String()),
			)
			defer failpoint.DefaultManager.Reset()

			dumpErrCh := make(chan error, 1)
			go func() { dumpErrCh <- md.Dump() }()

			fp, ok := failpoint.DefaultManager.Get(failpoint.PauseUntilResumed)
			require.True(t, ok, "should find the PauseUntilResumed failpoint")
			require.NoError(t, fp.Reached(context.TODO()))
			sessErr1 := session.Database("config").
				CreateCollection(ctx, "localReshardingOperations.donor")
			sessErr2 := session.Database("config").
				Collection("localReshardingOperations.donor").
				Drop(ctx)
			fp.Signal()

			err = <-dumpErrCh

			require.Error(
				t,
				err,
				"should refuse to dump while resharding is created during the oplog dump",
			)
			assert.Contains(
				t,
				err.Error(),
				oplogErrorMsg,
				"should explain that resharding is in progress",
			)
			assert.NoError(
				t,
				sessErr1,
				"should create the localReshardingOperations.donor collection",
			)
			assert.NoError(
				t,
				sessErr2,
				"should drop the localReshardingOperations.donor collection",
			)
		},
	)

	t.Run(
		"dump should fail if config.localReshardingOperations.recipient created in oplog",
		func(t *testing.T) {
			md := newFailDuringReshardingMongoDump(t)
			require.NoError(
				t,
				failpoint.DefaultManager.Parse(failpoint.PauseUntilResumed.String()),
			)
			defer failpoint.DefaultManager.Reset()

			dumpErrCh := make(chan error, 1)
			go func() { dumpErrCh <- md.Dump() }()

			fp, ok := failpoint.DefaultManager.Get(failpoint.PauseUntilResumed)
			require.True(t, ok, "should find the PauseUntilResumed failpoint")
			require.NoError(t, fp.Reached(context.TODO()))
			sessErr1 := session.Database("config").
				CreateCollection(ctx, "localReshardingOperations.recipient")
			sessErr2 := session.Database("config").
				Collection("localReshardingOperations.recipient").
				Drop(ctx)
			fp.Signal()

			err = <-dumpErrCh

			require.Error(
				t,
				err,
				"should refuse to dump while resharding is created during the oplog dump",
			)
			assert.Contains(
				t,
				err.Error(),
				oplogErrorMsg,
				"should explain that resharding is in progress",
			)
			assert.NoError(
				t,
				sessErr1,
				"should create the localReshardingOperations.recipient collection",
			)
			assert.NoError(
				t,
				sessErr2,
				"should drop the localReshardingOperations.recipient collection",
			)
		},
	)
}

// builds a fresh MongoDump instance per subtest, since Dump closes the
// underlying session provider and a shared instance would only work once.
func newFailDuringReshardingMongoDump(t *testing.T) *MongoDump {
	t.Helper()

	err := setUpMongoDumpTestData(t)
	require.NoError(t, err, "should set up the test data")

	md, err := simpleMongoDumpInstance()
	require.NoError(t, err, "should build a MongoDump instance")

	md.OutputOptions.Oplog = true
	md.ToolOptions.Namespace = &options.Namespace{}
	err = md.Init()
	require.NoError(t, err, "should initialize the MongoDump instance")

	return md
}

func TestOptionsOrderIsPreserved(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	log.SetWriter(io.Discard)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)

	collName := "students"
	viewName := "studentsView"

	pipeline := bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "year", Value: "$year"},
				{Key: "name", Value: "$name"},
			}},
			{Key: "highest", Value: bson.D{
				{Key: "$max", Value: "$score"},
			}},
		}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{
			{Key: "year", Value: 1},
			{Key: "sID", Value: -1},
			{Key: "name", Value: 1},
			{Key: "score", Value: 1},
		}}},
	}

	createViewCmd := bson.D{
		{"create", viewName},
		{"viewOn", collName},
		{"pipeline", pipeline},
	}

	var result bson.D
	err = sessionProvider.Run(createViewCmd, &result, testDB)
	require.NoError(t, err)

	// The check should be run a few times due to the probabilistic nature
	// of TOOLS-3411
	for i := 0; i < 10; i++ {
		dumpAndCheckPipelineOrder(t, collName, pipeline)
	}

	err = tearDownMongoDumpTestData(t)
	require.NoError(t, err)
}

func dumpAndCheckPipelineOrder(t *testing.T, collName string, pipeline bson.A) {
	md, err := simpleMongoDumpInstance()
	require.NoError(t, err)

	md.ToolOptions.DB = testDB
	md.OutputOptions.Out = "dump"

	require.NoError(t, md.Init())
	require.NoError(t, md.Dump())

	path, err := os.Getwd()
	require.NoError(t, err)

	dumpDir := filepath.FromSlash(filepath.Join(path, "dump"))
	dumpDBDir := filepath.FromSlash(filepath.Join(dumpDir, testDB))
	require.True(t, fileDirExists(dumpDir))
	require.True(t, fileDirExists(dumpDBDir))

	metaFiles, err := getMatchingFiles(dumpDBDir, "studentsView\\.metadata\\.json")
	require.NoError(t, err)
	require.Equal(t, len(metaFiles), 1)

	metaFile, err := os.Open(filepath.FromSlash(filepath.Join(dumpDBDir, metaFiles[0])))

	require.NoError(t, err)
	contents, err := io.ReadAll(metaFile)
	require.NoError(t, err)

	var bsonResult bson.D
	err = bson.UnmarshalExtJSON(contents, true, &bsonResult)
	require.NoError(t, err)
	options, err := bsonutil.FindSubdocumentByKey("options", &bsonResult)
	require.NoError(t, err)

	assertBSONEqual(t, options, bson.D{
		{Key: "viewOn", Value: collName},
		{Key: "pipeline", Value: pipeline},
	})

	os.RemoveAll(dumpDir)
	metaFile.Close()
}

// TestBrokenPipe verifies that mongodump handles a broken pipe gracefully
// (exits with a write error rather than being killed by SIGPIPE).
func TestBrokenPipe(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongodump_broken_pipe_test"
		collName = "docs"
	)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)
	client, err := sessionProvider.GetSession()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Database(dbName).Drop(context.Background())
	})

	// Insert 2000 docs so the archive output exceeds the pipe buffer.
	docs := make([]any, 2000)
	for i := range 2000 {
		docs[i] = bson.D{{"_id", int32(i)}, {"data", strings.Repeat("x", 500)}}
	}
	_, err = client.Database(dbName).Collection(collName).InsertMany(context.Background(), docs)
	require.NoError(t, err)

	args := append(
		[]string{"run", filepath.Join("..", "mongodump", "main")},
		testutil.GetBareArgs()...,
	)
	args = append(args, "--db", dbName, "--archive=-")
	testutil.AssertBrokenPipeHandled(t, exec.Command("go", args...))
}
