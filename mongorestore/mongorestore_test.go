// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/mongodb/mongo-tools-common/bsonutil"
	"go.mongodb.org/mongo-driver/mongo"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/testtype"
	"github.com/mongodb/mongo-tools-common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
	driverOpt "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const (
	mioSoeFile = "testdata/10k1dup10k.bson"
)

func init() {
	// bump up the verbosity to make checking debug log output possible
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

func getRestoreWithArgs(additionalArgs ...string) (*MongoRestore, error) {
	opts, err := ParseOptions(append(testutil.GetBareArgs(), additionalArgs...), "", "")
	if err != nil {
		return nil, fmt.Errorf("error parsing args: %v", err)
	}

	restore, err := New(opts)
	if err != nil {
		return nil, fmt.Errorf("error making new instance of mongorestore: %v", err)
	}

	return restore, nil
}

func TestMongorestore(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		session, _ := restore.SessionProvider.GetSession()

		db := session.Database("db1")
		Convey("and majority is used as the default write concern", func() {
			So(db.WriteConcern(), ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		c1 := db.Collection("c1")
		c1.Drop(nil)
		Convey("and an explicit target restores from that dump directory", func() {
			restore.TargetDirectory = "testdata/testdirs"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 100)
			So(result.Failures, ShouldEqual, 0)
			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)
		})

		Convey("and an target of '-' restores from standard input", func() {
			bsonFile, err := os.Open("testdata/testdirs/db1/c1.bson")
			restore.NSOptions.Collection = "c1"
			restore.NSOptions.DB = "db1"
			So(err, ShouldBeNil)
			restore.InputReader = bsonFile
			restore.TargetDirectory = "-"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)
		})
	})
}

func TestMongorestoreCantPreserveUUID(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "3.6"); err != nil || cmp >= 0 {
		t.Skip("Requires server with FCV less than 3.6")
	}

	Convey("PreserveUUID restore with incompatible destination FCV errors", func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			PreserveUUIDOption,
			DropOption,
			"testdata/oplogdump",
		}
		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		result := restore.Restore()
		So(result.Err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "target host does not support --preserveUUID")
	})
}

func TestMongorestorePreserveUUID(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "3.6"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 3.6 or later")
	}

	// From mongorestore/testdata/oplogdump/db1/c1.metadata.json
	originalUUID := "699f503df64b4aa8a484a8052046fa3a"

	Convey("With a test MongoRestore", t, func() {
		c1 := session.Database("db1").Collection("c1")
		c1.Drop(nil)

		Convey("normal restore gives new UUID", func() {
			args := []string{
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
				"testdata/oplogdump",
			}
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 5)
			info, err := db.GetCollectionInfo(c1)
			So(err, ShouldBeNil)
			So(info.GetUUID(), ShouldNotEqual, originalUUID)
		})

		Convey("PreserveUUID restore without drop errors", func() {
			args := []string{
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
				PreserveUUIDOption,
				"testdata/oplogdump",
			}
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			So(result.Err, ShouldNotBeNil)
			So(result.Err.Error(), ShouldContainSubstring, "cannot specify --preserveUUID without --drop")
		})

		Convey("PreserveUUID with drop preserves UUID", func() {
			args := []string{
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
				PreserveUUIDOption,
				DropOption,
				"testdata/oplogdump",
			}
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 5)
			info, err := db.GetCollectionInfo(c1)
			So(err, ShouldBeNil)
			So(info.GetUUID(), ShouldEqual, originalUUID)
		})

		Convey("PreserveUUID on a file without UUID metadata errors", func() {
			args := []string{
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
				PreserveUUIDOption,
				DropOption,
				"testdata/testdirs",
			}
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			So(result.Err, ShouldNotBeNil)
			So(result.Err.Error(), ShouldContainSubstring, "--preserveUUID used but no UUID found")
		})

	})
}

// generateTestData creates the files used in TestMongorestoreMIOSOE
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
		buf, err := bson.Marshal(bson.D{{"_id", i}})
		if err != nil {
			return err
		}
		_, err = w.Write(buf)
		if err != nil {
			return err
		}
	}

	// 1 duplicate _id
	buf, err := bson.Marshal(bson.D{{"_id", 5}})
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	if err != nil {
		return err
	}

	// 10k unique _id's
	for i := 10001; i < 20001; i++ {
		buf, err := bson.Marshal(bson.D{{"_id", i}})
		if err != nil {
			return err
		}
		_, err = w.Write(buf)
		if err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}

type CollectionInfo struct {
	Options *bson.D
}

type ByIndexName []bson.D

func (n ByIndexName) Len() int {
	return len(n)
}

func (n ByIndexName) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

func (n ByIndexName) Less(i, j int) bool {
	return getIndexName(n[i]) < getIndexName(n[j])
}

// removeKey removes the given key. Returns the removed value and true if the
// key was found.
func removeKey(key string, document *bson.D) (interface{}, bool) {
	if document == nil {
		return nil, false
	}
	doc := *document
	for i, elem := range doc {
		if elem.Key == key {
			// Remove this key.
			*document = append(doc[:i], doc[i+1:]...)
			return elem.Value, true
		}
	}
	return nil, false
}

// getCollectionDocs returns a slice of all documents in a collection.
func getCollectionDocs(coll *mongo.Collection) ([]bson.D, error) {
	c, err := coll.Find(context.Background(), bson.D{}, driverOpt.Find().SetSort(bson.D{{"_id", 1}}))
	if err != nil {
		return nil, err
	}

	defer c.Close(context.Background())
	var docs []bson.D
	for c.Next(context.Background()) {
		var doc bson.D
		if err = c.Decode(&doc); err != nil {
			return nil, err
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func findStringValueByKey(keyName string, document *bson.D) (string, error) {
	value, err := bsonutil.FindValueByKey(keyName, document)
	if err != nil {
		return "", err
	}
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("field present, but is not a string: %v", value)
	}
	return str, nil
}

func getIndexName(index bson.D) string {
	name, err := findStringValueByKey("name", &index)
	if err != nil {
		return ""
	}
	return name
}

// isViewError checks if err is an "CommandNotSupportedOnView" error.
func isViewError(err error) bool {
	e, ok := err.(mongo.CommandError)
	return ok && e.Code == 166
}

// bsonDToM converts the given bson.D to bson.M.
func bsonDToM(d *bson.D) *bson.M {
	if d == nil {
		return nil
	}
	m := make(bson.M)
	for _, elem := range *d {
		m[elem.Key] = elem.Value
	}
	return &m
}

// fullCollectionName returns the full namespace for a collection (dbName.collectionName)
func fullCollectionName(c *mongo.Collection) string {
	return fmt.Sprintf("%s.%s", c.Database().Name(), c.Name())
}

func getIndexDocumentsForCollection(c *mongo.Collection) ([]bson.D, error) {
	var indexes []bson.D
	indexesIter, err := c.Indexes().List(context.Background())
	if err != nil {
		if isViewError(err) {
			return indexes, nil
		}
		return nil, err
	}
	for indexesIter.Next(context.Background()) {
		var index bson.D
		if err := indexesIter.Decode(&index); err != nil {
			return nil, err
		}

		indexes = append(indexes, index)
	}
	sort.Sort(ByIndexName(indexes))
	return indexes, nil
}

// AssertBsonDEqualUnordered, asserts two bson.Ds are equal ignoring the
// order of the top level fields. Useful for comparing indexes.
func assertBsonDEqualUnordered(expected, actual *bson.D) {
	_, err := bson.MarshalExtJSON(expected, true, true)
	So(err, ShouldBeNil)

	_, err = bson.MarshalExtJSON(actual, true, true)
	So(err, ShouldBeNil)

	expectedM := bsonDToM(expected)
	actualM := bsonDToM(actual)
	So(reflect.DeepEqual(expectedM, actualM), ShouldBeTrue)
}

func assertIndexesEqual(source, dest *mongo.Collection, destinationVersion db.Version) {
	sourceIndexes, err := getIndexDocumentsForCollection(source)
	So(err, ShouldBeNil)

	destIndexes, err := getIndexDocumentsForCollection(dest)
	So(err, ShouldBeNil)
	So(len(sourceIndexes), ShouldEqual, len(destIndexes))

	for i := 0; i < len(sourceIndexes) && i < len(destIndexes); i++ {
		sourceIndex := sourceIndexes[i]
		destIndex := destIndexes[i]

		// Do not compare the "v" field and textIndexVersion field.
		// remove the "v" flag of the _id_ index which may haven been
		// created with a different version on the destination.
		// remove the "textIndexVersion" key from both source and destination indexes during comparison
		if destinationVersion.Cmp(db.Version{4, 2, 0}) != 0 {
			name, err := findStringValueByKey("name", &sourceIndex)
			So(err, ShouldNotBeNil)

			if name == "_id_" {
				// "v" should always be the first field returned in an index spec.
				So("v", ShouldEqual, sourceIndex[0].Key, "source index first field name")
				So("v", ShouldEqual, destIndex[0].Key, "dest index first field name")
				sourceIndex = sourceIndex[1:]
				destIndex = destIndex[1:]
			}

			removeKey("textIndexVersion", &sourceIndex)
			removeKey("textIndexVersion", &destIndex)
		}

		assertBsonDEqualUnordered(&sourceIndex, &destIndex)
	}
}

// getCollections returns an iterator to the listCollections output for the
// given database.
func getCollections(database *mongo.Database, name string) (*mongo.Cursor, error) {
	var filter bson.D
	if len(name) > 0 {
		filter = bson.D{{"name", name}}
	}

	return database.ListCollections(context.Background(), filter)
}


// getCollectionInfo returns the listCollections output for the
// given collection.
func getCollectionInfo(c *mongo.Collection, ctx context.Context) (*CollectionInfo, error) { // parameterize db and coll names!
	cursor, err := getCollections(c.Database(), c.Name())
	So(err, ShouldBeNil)

	defer cursor.Close(ctx)

	_ = cursor.Next(ctx)

	collInfo := &CollectionInfo{}
	if err = cursor.Decode(&collInfo); err != nil {
		return collInfo, err
	}

	return collInfo, cursor.Err()
}

// assertBsonDEqual asserts two bson.Ds are equal.
func assertBsonDEqual(expected, actual *bson.D, label string) {
	if expected == nil {
		So(actual, ShouldBeNil)
		return
	}

	expectedJSON, err := bson.MarshalExtJSON(expected, true, true)
	So(err, ShouldBeNil)

	actualJSON, err := bson.MarshalExtJSON(actual, true, true)
	So(err, ShouldBeNil)

	So(bytes.Equal(expectedJSON, actualJSON), ShouldBeTrue)
}

func assertCollectionEqual(t *testing.T, source, dest *mongo.Collection, session *mongo.Client, ctx context.Context) {
	t.Helper()
	// Assert that the collection at the destination is created with the same options.
	sourceCollInfo, err := getCollectionInfo(source, ctx)
	So(err, ShouldBeNil)

	destinationCollInfo, err := getCollectionInfo(dest, ctx)
	So(err, ShouldBeNil)

	args := []string{
		OplogReplayOption, "1",
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	So(err, ShouldBeNil)

	assertBsonDEqual(sourceCollInfo.Options, destinationCollInfo.Options,
		fmt.Sprintf("collection options for destination collection %s", fullCollectionName(dest)))

	// Assert that all documents in the collections are the same.
	sourceDocs, err := getCollectionDocs(source)
	So(err, ShouldBeNil)

	destDocs, err := getCollectionDocs(dest)
	So(err, ShouldBeNil)

	So(len(sourceDocs), ShouldEqual, len(destDocs))
	for i := 0; i < len(sourceDocs) && i < len(destDocs); i++ {
		assertBsonDEqual(&sourceDocs[i], &destDocs[i],
			fmt.Sprintf("destination collection %s, document: %v", fullCollectionName(dest), i))
	}
	assertIndexesEqual(source, dest, restore.serverVersion)
}

// test --maintainInsertionOrder and --stopOnError behavior
func TestMongorestoreMIOSOE(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	if err := generateTestData(); err != nil {
		t.Fatalf("Couldn't generate test data %v", err)
	}

	client, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}
	database := client.Database("miodb")
	coll := database.Collection("mio")

	Convey("default restore ignores dup key errors", t, func() {
		restore, err := getRestoreWithArgs(mioSoeFile,
			CollectionOption, coll.Name(),
			DBOption, database.Name(),
			DropOption)
		So(err, ShouldBeNil)
		So(restore.OutputOptions.MaintainInsertionOrder, ShouldBeFalse)

		result := restore.Restore()
		So(result.Err, ShouldBeNil)
		So(result.Successes, ShouldEqual, 20000)
		So(result.Failures, ShouldEqual, 1)

		count, err := coll.CountDocuments(nil, bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 20000)
	})

	Convey("--maintainInsertionOrder stops exactly on dup key errors", t, func() {
		restore, err := getRestoreWithArgs(mioSoeFile,
			CollectionOption, coll.Name(),
			DBOption, database.Name(),
			DropOption,
			MaintainInsertionOrderOption)
		So(err, ShouldBeNil)
		So(restore.OutputOptions.MaintainInsertionOrder, ShouldBeTrue)
		So(restore.OutputOptions.NumInsertionWorkers, ShouldEqual, 1)

		result := restore.Restore()
		So(result.Err, ShouldNotBeNil)
		So(result.Successes, ShouldEqual, 10000)
		So(result.Failures, ShouldEqual, 1)

		count, err := coll.CountDocuments(nil, bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 10000)
	})

	Convey("--stopOnError stops on dup key errors", t, func() {
		restore, err := getRestoreWithArgs(mioSoeFile,
			CollectionOption, coll.Name(),
			DBOption, database.Name(),
			DropOption,
			StopOnErrorOption,
			NumParallelCollectionsOption, "1")
		So(err, ShouldBeNil)
		So(restore.OutputOptions.StopOnError, ShouldBeTrue)

		result := restore.Restore()
		So(result.Err, ShouldNotBeNil)
		So(result.Successes, ShouldAlmostEqual, 10000, restore.OutputOptions.BulkBufferSize)
		So(result.Failures, ShouldEqual, 1)

		count, err := coll.CountDocuments(nil, bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldAlmostEqual, 10000, restore.OutputOptions.BulkBufferSize)
	})

	_ = database.Drop(nil)
}

func TestDeprecatedIndexOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		session, _ = restore.SessionProvider.GetSession()

		db := session.Database("indextest")

		coll := db.Collection("test_collection")
		coll.Drop(nil)
		defer func() {
			coll.Drop(nil)
		}()
		Convey("Creating index with invalid option should throw error", func() {
			restore.TargetDirectory = "testdata/indextestdump"
			result := restore.Restore()
			So(result.Err, ShouldNotBeNil)
			So(result.Err.Error(), ShouldStartWith, `indextest.test_collection: error creating indexes for indextest.test_collection: createIndex error: (InvalidIndexSpecificationOption)`)

			So(result.Successes, ShouldEqual, 100)
			So(result.Failures, ShouldEqual, 0)
			count, err := coll.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)
		})

		coll.Drop(nil)

		args = []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			ConvertLegacyIndexesOption, "true",
		}

		restore, err = getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		Convey("Creating index with invalid option and --ignoreInvalidIndexOptions should succeed", func() {
			restore.TargetDirectory = "testdata/indextestdump"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			So(result.Successes, ShouldEqual, 100)
			So(result.Failures, ShouldEqual, 0)
			count, err := coll.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)
		})
	})
}

func TestRestoreUsersOrRoles(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		session, _ = restore.SessionProvider.GetSession()

		db := session.Database("admin")

		Convey("Restoring users and roles should drop tempusers and temproles", func() {
			restore.TargetDirectory = "testdata/usersdump"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			adminCollections, err := db.ListCollectionNames(context.Background(), bson.M{})
			So(err, ShouldBeNil)

			for _, collName := range adminCollections {
				So(collName, ShouldNotEqual, "tempusers")
				So(collName, ShouldNotEqual, "temproles")
			}
		})
	})
}

func TestKnownCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		session, _ = restore.SessionProvider.GetSession()
		db := session.Database("test")
		defer func() {
			db.Collection("foo").Drop(nil)
		}()

		Convey("Once collection foo has been restored, it should exist in restore.knownCollections", func() {
			restore.TargetDirectory = "testdata/foodump"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			var namespaceExistsInCache bool
			if cols, ok := restore.knownCollections["test"]; ok {
				for _, collName := range cols {
					if collName == "foo" {
						namespaceExistsInCache = true
					}
				}
			}
			So(namespaceExistsInCache, ShouldBeTrue)
		})
	})
}

func TestFixHashedIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	type indexRes struct {
		Key bson.D
	}

	Convey("Test MongoRestore with hashed indexes and --fixHashedIndexes", t, func() {
		args := []string{
			FixDottedHashedIndexesOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		session, _ = restore.SessionProvider.GetSession()
		db := session.Database("testdata")

		defer func() {
			db.Collection("hashedIndexes").Drop(nil)
		}()

		Convey("The index for a.b should be changed from 'hashed' to 1, since it is dotted", func() {
			restore.TargetDirectory = "testdata/hashedIndexes.bson"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			indexes := db.Collection("hashedIndexes").Indexes()
			c, err := indexes.List(context.Background())
			So(err, ShouldBeNil)
			var res indexRes

			for c.Next(context.Background()) {
				err := c.Decode(&res)
				So(err, ShouldBeNil)
				for _, key := range res.Key {
					if key.Key == "b" {
						So(key.Value, ShouldEqual, "hashed")
					} else if key.Key == "a.a" {
						So(key.Value, ShouldEqual, 1)
					} else if key.Key == "a.b" {
						So(key.Value, ShouldEqual, 1)
					} else if key.Key != "_id" {
						t.Fatalf("Unexepected Index: %v", key.Key)
					}
				}
			}
		})
	})

	Convey("Test MongoRestore with hashed indexes without --fixHashedIndexes", t, func() {
		args := []string{}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		session, _ = restore.SessionProvider.GetSession()
		db := session.Database("testdata")

		defer func() {
			db.Collection("hashedIndexes").Drop(nil)
		}()

		Convey("All indexes should be unchanged", func() {
			restore.TargetDirectory = "testdata/hashedIndexes.bson"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			indexes := db.Collection("hashedIndexes").Indexes()
			c, err := indexes.List(context.Background())
			So(err, ShouldBeNil)
			var res indexRes

			for c.Next(context.Background()) {
				err := c.Decode(&res)
				So(err, ShouldBeNil)
				for _, key := range res.Key {
					if key.Key == "b" {
						So(key.Value, ShouldEqual, "hashed")
					} else if key.Key == "a.a" {
						So(key.Value, ShouldEqual, 1)
					} else if key.Key == "a.b" {
						So(key.Value, ShouldEqual, "hashed")
					} else if key.Key != "_id" {
						t.Fatalf("Unexepected Index: %v", key.Key)
					}
				}
			}
		})
	})
}

func TestAutoIndexIdLocalDB(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("Test MongoRestore with {autoIndexId: false} in a local database's collection", t, func() {
		dbName := session.Database("local")

		// Drop the collection to clean up resources
		defer dbName.Collection("test_auto_idx").Drop(ctx)

		var args []string

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		restore.TargetDirectory = "testdata/local/test_auto_idx.bson"
		result := restore.Restore()
		So(result.Err, ShouldBeNil)

		// Find the collection
		filter := bson.D{{"name", "test_auto_idx"}}
		cursor, err := session.Database("local").ListCollections(ctx, filter)
		So(err, ShouldBeNil)

		defer cursor.Close(ctx)

		documentExists := cursor.Next(ctx)
		So(documentExists, ShouldBeTrue)

		var collInfo struct {
			Options bson.M
		}
		err = cursor.Decode(&collInfo)
		So(err, ShouldBeNil)

		So(collInfo.Options["autoIndexId"], ShouldBeFalse)
	})
}

func TestAutoIndexIdNonLocalDB(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("Test MongoRestore with {autoIndexId: false} in a non-local database's collection", t, func() {
		Convey("Do not set --preserveUUID\n", func() {
			dbName := session.Database("testdata")

			// Drop the collection to clean up resources
			defer dbName.Collection("test_auto_idx").Drop(ctx)

			var args []string

			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			restore.TargetDirectory = "testdata/test_auto_idx.bson"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			// Find the collection
			filter := bson.D{{"name", "test_auto_idx"}}
			cursor, err := session.Database("testdata").ListCollections(ctx, filter)
			So(err, ShouldBeNil)

			defer cursor.Close(ctx)

			documentExists := cursor.Next(ctx)
			So(documentExists, ShouldBeTrue)

			var collInfo struct {
				Options bson.M
			}
			err = cursor.Decode(&collInfo)
			So(err, ShouldBeNil)

			Convey("{autoIndexId: false} should be flipped to true if server version >= 4.0", func() {
				if restore.serverVersion.GTE(db.Version{4, 0, 0}) {
					So(collInfo.Options["autoIndexId"], ShouldBeTrue)
				} else {
					So(collInfo.Options["autoIndexId"], ShouldBeFalse)
				}
			})
		})
		dbName := session.Database("testdata")

		// Drop the collection to clean up resources
		defer dbName.Collection("test_auto_idx").Drop(ctx)

		args := []string{
			PreserveUUIDOption, "1",
			DropOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)

		if restore.serverVersion.GTE(db.Version{4, 0, 0}) {
			Convey("Set --preserveUUID if server version >= 4.0\n", func() {
				restore.TargetDirectory = "testdata/test_auto_idx.bson"
				result := restore.Restore()
				So(result.Err, ShouldBeNil)

				// Find the collection
				filter := bson.D{{"name", "test_auto_idx"}}
				cursor, err := session.Database("testdata").ListCollections(ctx, filter)
				So(err, ShouldBeNil)

				defer cursor.Close(ctx)

				documentExists := cursor.Next(ctx)
				So(documentExists, ShouldBeTrue)

				var collInfo struct {
					Options bson.M
				}
				err = cursor.Decode(&collInfo)
				So(err, ShouldBeNil)

				Convey("{autoIndexId: false} should be flipped to true if server version >= 4.0", func() {
					if restore.serverVersion.GTE(db.Version{4, 0, 0}) {
						So(collInfo.Options["autoIndexId"], ShouldBeTrue)
					} else {
						So(collInfo.Options["autoIndexId"], ShouldBeFalse)
					}
				})
			})
		}
	})
}

// TestSkipSystemCollections asserts that certain system collections like "config.systems.sessions" and the transaction
// related tables aren't applied via applyops when replaying the oplog.
func TestSkipSystemCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	ctx := context.Background()

	Convey("With a test MongoRestore", t, func() {
		session.Database("db3").RunCommand(ctx, bson.D{
			{"create", "c1"},
		})

		args := []string{
			DirectoryOption, "testdata/oplog_partial_skips",
			OplogReplayOption,
			DropOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		c1 := session.Database("db3").Collection("c1")
		c1.Drop(nil)

		// Run mongorestore
		collBeforeRestore := session.Database("db3").Collection("c1")
		So(err, ShouldBeNil)

		result := restore.Restore()
		So(result.Err, ShouldBeNil)
		So(result.Failures, ShouldEqual, 0)

		// Verify restoration
		_, err = c1.CountDocuments(nil, bson.M{})
		So(err, ShouldBeNil)

		assertCollectionEqual(t, collBeforeRestore, session.Database("db3").Collection("c1"), session, ctx)
		session.Disconnect(context.Background())
	})
}
