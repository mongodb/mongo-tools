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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const (
	mioSoeFile     = "testdata/10k1dup10k.bson"
	longFilePrefix = "aVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery" +
		"VeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery" +
		"VeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery"
	longCollectionName = longFilePrefix +
		"LongCollectionNameConsistingOfExactlyTwoHundredAndFortySevenCharacters"
	longBsonName = longFilePrefix +
		"LongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.bson"
	longMetadataName = longFilePrefix +
		"LongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.metadata.json"
	longInvalidBson = longFilePrefix +
		"LongCollectionNameConsistingOfE%24someMadeUpInvalidHashString.bson"
	specialCharactersCollectionName = "cafés"
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

func TestDeprecatedDBAndCollectionOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	// As specified in TOOLS-2363, the --db and --collection options
	// are well-defined only for restoration of a single BSON file
	Convey("The proper warning message is issued if --db and --collection "+
		"are used in a case where they are deprecated", t, func() {
		// Hacky way of looking at the application log at test-time

		// Ideally, we would be able to use some form of explicit dependency
		// injection to specify the sink for the parsing/validation log. However,
		// the validation logic here is coupled with the mongorestore.MongoRestore
		// type, which does not support such an injection.

		var buffer bytes.Buffer

		log.SetWriter(&buffer)
		defer log.SetWriter(os.Stderr)

		Convey("and no warning is issued in the well-defined case", func() {
			// No error and nothing written in the log
			args := []string{
				"testdata/hashedIndexes.bson",
				DBOption, "db1	",
				CollectionOption, "coll1",
			}

			restore, err := getRestoreWithArgs(args...)
			if err != nil {
				t.Fatalf("Cannot bootstrap test harness: %v", err.Error())
			}
			defer restore.Close()

			err = restore.ParseAndValidateOptions()

			So(err, ShouldBeNil)
			So(buffer.String(), ShouldBeEmpty)
		})

		Convey("and a warning is issued in the deprecated case", func() {
			// No error and some kind of warning message in the log
			args := []string{
				DBOption, "db1",
				CollectionOption, "coll1",
			}

			restore, err := getRestoreWithArgs(args...)
			if err != nil {
				t.Fatalf("Cannot bootstrap test harness: %v", err.Error())
			}
			defer restore.Close()

			err = restore.ParseAndValidateOptions()

			So(err, ShouldBeNil)
			So(buffer.String(), ShouldContainSubstring, deprecatedDBAndCollectionsOptionsWarning)
		})
	})
}

func TestMongorestore(t *testing.T) {
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
		defer restore.Close()

		db := session.Database("db1")
		Convey("and majority is used as the default write concern", func() {
			So(db.WriteConcern(), ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		c1 := db.Collection("c1") // 100 documents
		c1.Drop(nil)
		c2 := db.Collection("c2") // 0 documents
		c2.Drop(nil)
		c3 := db.Collection("c3") // 0 documents
		c3.Drop(nil)
		c4 := db.Collection("c4") // 10 documents
		c4.Drop(nil)

		Convey("and an explicit target restores from that dump directory", func() {
			restore.TargetDirectory = "testdata/testdirs"

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 110)
			So(result.Failures, ShouldEqual, 0)

			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)

			count, err = c4.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("and an target of '-' restores from standard input", func() {
			bsonFile, err := os.Open("testdata/testdirs/db1/c1.bson")
			So(err, ShouldBeNil)

			restore.ToolOptions.Namespace.Collection = "c1"
			restore.ToolOptions.Namespace.DB = "db1"
			restore.InputReader = bsonFile
			restore.TargetDirectory = "-"

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)
		})

		Convey("and specifying an nsExclude option", func() {
			restore.TargetDirectory = "testdata/testdirs"
			restore.NSOptions.NSExclude = make([]string, 1)
			restore.NSOptions.NSExclude[0] = "db1.c1"

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = c4.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("and specifying an nsInclude option", func() {
			restore.TargetDirectory = "testdata/testdirs"
			restore.NSOptions.NSInclude = make([]string, 1)
			restore.NSOptions.NSInclude[0] = "db1.c4"

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := c1.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = c4.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("and specifying nsFrom and nsTo options", func() {
			restore.TargetDirectory = "testdata/testdirs"

			restore.NSOptions.NSFrom = make([]string, 1)
			restore.NSOptions.NSFrom[0] = "db1.c1"
			restore.NSOptions.NSTo = make([]string, 1)
			restore.NSOptions.NSTo[0] = "db1.c1renamed"

			c1renamed := db.Collection("c1renamed")
			c1renamed.Drop(nil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 110)
			So(result.Failures, ShouldEqual, 0)

			count, err := c1renamed.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 100)

			count, err = c4.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})
	})
}

func TestMongoRestoreSpecialCharactersCollectionNames(t *testing.T) {
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
		defer restore.Close()

		db := session.Database("db1")

		specialCharacterCollection := db.Collection(specialCharactersCollectionName)
		specialCharacterCollection.Drop(nil)

		Convey("and --nsInclude a collection name with special characters", func() {
			restore.TargetDirectory = "testdata/specialcharacter"
			restore.NSOptions.NSInclude = make([]string, 1)
			restore.NSOptions.NSInclude[0] = "db1." + specialCharactersCollectionName

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 1)
			So(result.Failures, ShouldEqual, 0)

			count, err := specialCharacterCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("and --nsExclude a collection name with special characters", func() {
			restore.TargetDirectory = "testdata/specialcharacter"
			restore.NSOptions.NSExclude = make([]string, 1)
			restore.NSOptions.NSExclude[0] = "db1." + specialCharactersCollectionName
			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 0)

			count, err := specialCharacterCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("and --nsTo a collection name without special characters "+
			"--nsFrom a collection name with special characters", func() {
			restore.TargetDirectory = "testdata/specialcharacter"
			restore.NSOptions.NSFrom = make([]string, 1)
			restore.NSOptions.NSFrom[0] = "db1." + specialCharactersCollectionName
			restore.NSOptions.NSTo = make([]string, 1)
			restore.NSOptions.NSTo[0] = "db1.aCollectionNameWithoutSpecialCharacters"

			standardCharactersCollection := db.Collection("aCollectionNameWithoutSpecialCharacters")
			standardCharactersCollection.Drop(nil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 1)
			So(result.Failures, ShouldEqual, 0)

			count, err := standardCharactersCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("and --nsTo a collection name with special characters "+
			"--nsFrom a collection name with special characters", func() {
			restore.TargetDirectory = "testdata/specialcharacter"
			restore.NSOptions.NSFrom = make([]string, 1)
			restore.NSOptions.NSFrom[0] = "db1." + specialCharactersCollectionName
			restore.NSOptions.NSTo = make([]string, 1)
			restore.NSOptions.NSTo[0] = "db1.aCollectionNameWithSpećiálCharacters"

			standardCharactersCollection := db.Collection("aCollectionNameWithSpećiálCharacters")
			standardCharactersCollection.Drop(nil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 1)
			So(result.Failures, ShouldEqual, 0)

			count, err := standardCharactersCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})
	})
}

func TestMongorestoreLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 4.4 or later")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		db := session.Database("db1")
		Convey("and majority is used as the default write concern", func() {
			So(db.WriteConcern(), ShouldResemble, writeconcern.New(writeconcern.WMajority()))
		})

		longCollection := db.Collection(longCollectionName)
		longCollection.Drop(nil)

		Convey("and an explicit target restores truncated files from that dump directory", func() {
			restore.TargetDirectory = "testdata/longcollectionname"

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 1)
			So(result.Failures, ShouldEqual, 0)

			count, err := longCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("and an target of '-' restores truncated files from standard input", func() {
			longBsonFile, err := os.Open("testdata/longcollectionname/db1/" + longBsonName)
			So(err, ShouldBeNil)

			restore.ToolOptions.Namespace.Collection = longCollectionName
			restore.ToolOptions.Namespace.DB = "db1"
			restore.InputReader = longBsonFile
			restore.TargetDirectory = "-"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			count, err := longCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("and specifying an nsExclude option", func() {
			restore.TargetDirectory = "testdata/longcollectionname"
			restore.NSOptions.NSExclude = make([]string, 1)
			restore.NSOptions.NSExclude[0] = "db1." + longCollectionName

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 0)

			count, err := longCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("and specifying an nsInclude option", func() {
			restore.TargetDirectory = "testdata/longcollectionname"
			restore.NSOptions.NSInclude = make([]string, 1)
			restore.NSOptions.NSInclude[0] = "db1." + longCollectionName

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 1)
			So(result.Failures, ShouldEqual, 0)

			count, err := longCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("and specifying nsFrom and nsTo options", func() {
			restore.TargetDirectory = "testdata/longcollectionname"
			restore.NSOptions.NSFrom = make([]string, 1)
			restore.NSOptions.NSFrom[0] = "db1." + longCollectionName
			restore.NSOptions.NSTo = make([]string, 1)
			restore.NSOptions.NSTo[0] = "db1.aMuchShorterCollectionName"

			shortCollection := db.Collection("aMuchShorterCollectionName")
			shortCollection.Drop(nil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 1)
			So(result.Failures, ShouldEqual, 0)

			count, err := shortCollection.CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})
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
			defer restore.Close()

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
			defer restore.Close()

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
			defer restore.Close()

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
			defer restore.Close()

			result := restore.Restore()
			So(result.Err, ShouldBeNil)
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
		defer restore.Close()
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
		defer restore.Close()
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
		defer restore.Close()
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
		defer restore.Close()

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
			So(result.Err.Error(), ShouldStartWith, `indextest.test_collection: error creating indexes for indextest.test_collection: createIndex error:`)

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
		defer restore.Close()

		Convey("Creating index with invalid option and --convertLegacyIndexes should succeed", func() {
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

// TestFixDuplicatedLegacyIndexes restores two indexes with --convertLegacyIndexes flag, {foo: ""} and {foo: 1}
// Only one index {foo: 1} should be created
func TestFixDuplicatedLegacyIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "3.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 3.4 or later")
	}
	Convey("With a test MongoRestore", t, func() {
		args := []string{
			ConvertLegacyIndexesOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		Convey("Index with duplicate key after convertLegacyIndexes should be skipped", func() {
			restore.TargetDirectory = "testdata/duplicate_index_key"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 0)
			So(err, ShouldBeNil)

			testDB := session.Database("indextest")
			defer func() {
				err = testDB.Drop(nil)
				if err != nil {
					t.Fatalf("Failed to drop test database testdata")
				}
			}()

			c, err := testDB.Collection("duplicate_index_key").Indexes().List(context.Background())
			So(err, ShouldBeNil)

			type indexRes struct {
				Name string
				Key  bson.D
			}

			indexKeys := make(map[string]bson.D)

			// two Indexes should be created in addition to the _id, foo and foo_2
			for c.Next(context.Background()) {
				var res indexRes
				err = c.Decode(&res)
				So(err, ShouldBeNil)
				So(len(res.Key), ShouldEqual, 1)
				indexKeys[res.Name] = res.Key
			}

			So(len(indexKeys), ShouldEqual, 3)

			var indexKey bson.D
			// Check that only one of foo_, foo_1, or foo_1.0 was created
			indexKeyFoo, ok := indexKeys["foo_"]
			indexKeyFoo1, ok1 := indexKeys["foo_1"]
			indexKeyFoo10, ok10 := indexKeys["foo_1.0"]

			So(ok || ok1 || ok10, ShouldBeTrue)

			if ok {
				So(ok1 || ok10, ShouldBeFalse)
				indexKey = indexKeyFoo
			}

			if ok1 {
				So(ok || ok10, ShouldBeFalse)
				indexKey = indexKeyFoo1
			}

			if ok10 {
				So(ok || ok1, ShouldBeFalse)
				indexKey = indexKeyFoo10
			}

			So(len(indexKey), ShouldEqual, 1)
			So(indexKey[0].Key, ShouldEqual, "foo")
			So(indexKey[0].Value, ShouldEqual, 1)

			indexKey, ok = indexKeys["foo_2"]
			So(ok, ShouldBeTrue)
			So(len(indexKey), ShouldEqual, 1)
			So(indexKey[0].Key, ShouldEqual, "foo")
			So(indexKey[0].Value, ShouldEqual, 2)
		})
	})
}

func TestDeprecatedIndexOptionsOn44FCV(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 4.4 or later")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		session, _ = restore.SessionProvider.GetSession()

		db := session.Database("indextest")

		// 4.4 removes the 'ns' field nested under the 'index' field in metadata.json
		coll := db.Collection("test_coll_no_index_ns")
		coll.Drop(nil)
		defer func() {
			coll.Drop(nil)
		}()

		args = []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			ConvertLegacyIndexesOption, "true",
		}

		restore, err = getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		Convey("Creating index with --convertLegacyIndexes and 4.4 FCV should succeed", func() {
			restore.TargetDirectory = "testdata/indexmetadata"
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

func TestLongIndexName(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		session, err := restore.SessionProvider.GetSession()
		So(err, ShouldBeNil)

		coll := session.Database("longindextest").Collection("test_collection")
		coll.Drop(nil)
		defer func() {
			coll.Drop(nil)
		}()

		if restore.serverVersion.LT(db.Version{4, 2, 0}) {
			Convey("Creating index with a full name longer than 127 bytes should fail (<4.2)", func() {
				restore.TargetDirectory = "testdata/longindextestdump"
				result := restore.Restore()
				So(result.Err, ShouldNotBeNil)
				So(result.Err.Error(), ShouldContainSubstring, "namespace is too long (max size is 127 bytes)")
			})
		} else {
			Convey("Creating index with a full name longer than 127 bytes should succeed (>=4.2)", func() {
				restore.TargetDirectory = "testdata/longindextestdump"
				result := restore.Restore()
				So(result.Err, ShouldBeNil)

				indexes := session.Database("longindextest").Collection("test_collection").Indexes()
				c, err := indexes.List(context.Background())
				So(err, ShouldBeNil)

				type indexRes struct {
					Name string
				}
				var names []string
				for c.Next(context.Background()) {
					var r indexRes
					err := c.Decode(&r)
					So(err, ShouldBeNil)
					names = append(names, r.Name)
				}
				So(len(names), ShouldEqual, 2)
				sort.Strings(names)
				So(names[0], ShouldEqual, "_id_")
				So(names[1], ShouldEqual, "a_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
			})
		}

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
		defer restore.Close()

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
		defer restore.Close()

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
		defer restore.Close()

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
		defer restore.Close()

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

		opts, err := ParseOptions(append(testutil.GetBareArgs()), "", "")
		So(err, ShouldBeNil)

		// Set retryWrites to false since it is unsupported on `local` db.
		retryWrites := false
		opts.RetryWrites = &retryWrites

		restore, err := New(opts)
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
			defer restore.Close()

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
		defer restore.Close()

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
	ctx := context.Background()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available")
	}

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.SkipNow()
	}

	sessionProvider.GetNodeType()

	Convey("With a test MongoRestore instance", t, func() {
		db3 := session.Database("db3")

		// Drop the collection to clean up resources
		defer db3.Collection("c1").Drop(ctx)

		args := []string{
			DirectoryOption, "testdata/oplog_partial_skips",
			OplogReplayOption,
			DropOption,
		}

		currentTS := uint32(time.Now().UTC().Unix())

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		So(result.Err, ShouldBeNil)

		Convey("applyOps should skip certain system-related collections during mongorestore", func() {
			queryObj := bson.D{
				{"$and",
					bson.A{
						bson.D{{"ts", bson.M{"$gte": primitive.Timestamp{T: currentTS, I: 1}}}},
						bson.D{{"$or", bson.A{
							bson.D{{"ns", primitive.Regex{Pattern: "^config.system.sessions*"}}},
							bson.D{{"ns", primitive.Regex{Pattern: "^config.cache.*"}}},
						}}},
					},
				},
			}

			cursor, err := session.Database("local").Collection("oplog.rs").Find(nil, queryObj, nil)
			So(err, ShouldBeNil)

			flag := cursor.Next(ctx)
			So(flag, ShouldBeFalse)

			cursor.Close(ctx)
		})
	})
}

// TestSkipStartAndAbortIndexBuild asserts that all "startIndexBuild" and "abortIndexBuild" oplog
// entries are skipped when restoring the oplog.
func TestSkipStartAndAbortIndexBuild(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available")
	}

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.SkipNow()
	}

	Convey("With a test MongoRestore instance", t, func() {
		testdb := session.Database("test")

		// Drop the collection to clean up resources
		defer testdb.Collection("skip_index_entries").Drop(ctx)

		// oplog.bson only has startIndexBuild and abortIndexBuild entries
		args := []string{
			DirectoryOption, "testdata/oplog_ignore_index",
			OplogReplayOption,
			DropOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		if restore.serverVersion.GTE(db.Version{4, 4, 0}) {
			// Run mongorestore
			dbLocal := session.Database("local")
			queryObj := bson.D{{
				"and", bson.A{
					bson.D{{"ns", bson.M{"$ne": "config.system.sessions"}}},
					bson.D{{"op", bson.M{"$ne": "n"}}},
				},
			}}

			countBeforeRestore, err := dbLocal.Collection("oplog.rs").CountDocuments(ctx, queryObj)
			So(err, ShouldBeNil)

			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			Convey("No new oplog entries should be recorded", func() {
				// Filter out no-ops
				countAfterRestore, err := dbLocal.Collection("oplog.rs").CountDocuments(ctx, queryObj)

				So(err, ShouldBeNil)
				So(countBeforeRestore, ShouldEqual, countAfterRestore)
			})
		}
	})
}

// TestcommitIndexBuild asserts that all "commitIndexBuild" are converted to creatIndexes commands
func TestCommitIndexBuild(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()
	testDB := "commit_index"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available")
	}

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV at least 4.4")
	}

	sessionProvider.GetNodeType()

	Convey("With a test MongoRestore instance", t, func() {
		testdb := session.Database(testDB)

		// Drop the collection to clean up resources
		defer testdb.Collection(testDB).Drop(ctx)

		args := []string{
			DirectoryOption, "testdata/commit_indexes_build",
			OplogReplayOption,
			DropOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		So(result.Err, ShouldBeNil)

		Convey("RestoreOplog() should convert commitIndexBuild op to createIndexes cmd and build index", func() {
			destColl := session.Database("commit_index").Collection("test")
			indexes, _ := destColl.Indexes().List(context.Background())

			type indexSpec struct {
				Name, NS                string
				Key                     bson.D
				Unique                  bool    `bson:",omitempty"`
				DropDups                bool    `bson:"dropDups,omitempty"`
				Background              bool    `bson:",omitempty"`
				Sparse                  bool    `bson:",omitempty"`
				Bits                    int     `bson:",omitempty"`
				Min                     float64 `bson:",omitempty"`
				Max                     float64 `bson:",omitempty"`
				BucketSize              float64 `bson:"bucketSize,omitempty"`
				ExpireAfter             int     `bson:"expireAfterSeconds,omitempty"`
				Weights                 bson.D  `bson:",omitempty"`
				DefaultLanguage         string  `bson:"default_language,omitempty"`
				LanguageOverride        string  `bson:"language_override,omitempty"`
				TextIndexVersion        int     `bson:"textIndexVersion,omitempty"`
				PartialFilterExpression bson.M  `bson:"partialFilterExpression,omitempty"`

				Collation bson.D `bson:"collation,omitempty"`
			}

			indexCnt := 0
			for indexes.Next(context.Background()) {
				var index indexSpec
				err := indexes.Decode(&index)
				So(err, ShouldBeNil)
				indexCnt++
			}
			// Should create 3 indexes: _id and two others
			So(indexCnt, ShouldEqual, 3)
		})
	})
}

// CreateIndexes oplog will be applied directly for versions < 4.4 and converted to createIndex cmd > 4.4
func TestCreateIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()
	testDB := "create_indexes"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available")
	}

	sessionProvider.GetNodeType()

	Convey("With a test MongoRestore instance", t, func() {
		testdb := session.Database(testDB)

		// Drop the collection to clean up resources
		defer testdb.Collection(testDB).Drop(ctx)

		args := []string{
			DirectoryOption, "testdata/create_indexes",
			OplogReplayOption,
			DropOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		So(result.Err, ShouldBeNil)

		Convey("RestoreOplog() should convert commitIndexBuild op to createIndexes cmd and build index", func() {
			destColl := session.Database("create_indexes").Collection("test")
			indexes, _ := destColl.Indexes().List(context.Background())

			type indexSpec struct {
				Name, NS                string
				Key                     bson.D
				Unique                  bool    `bson:",omitempty"`
				DropDups                bool    `bson:"dropDups,omitempty"`
				Background              bool    `bson:",omitempty"`
				Sparse                  bool    `bson:",omitempty"`
				Bits                    int     `bson:",omitempty"`
				Min                     float64 `bson:",omitempty"`
				Max                     float64 `bson:",omitempty"`
				BucketSize              float64 `bson:"bucketSize,omitempty"`
				ExpireAfter             int     `bson:"expireAfterSeconds,omitempty"`
				Weights                 bson.D  `bson:",omitempty"`
				DefaultLanguage         string  `bson:"default_language,omitempty"`
				LanguageOverride        string  `bson:"language_override,omitempty"`
				TextIndexVersion        int     `bson:"textIndexVersion,omitempty"`
				PartialFilterExpression bson.M  `bson:"partialFilterExpression,omitempty"`

				Collation bson.D `bson:"collation,omitempty"`
			}

			indexCnt := 0
			for indexes.Next(context.Background()) {
				var index indexSpec
				err := indexes.Decode(&index)
				So(err, ShouldBeNil)
				indexCnt++
			}
			// Should create 3 indexes: _id and two others
			So(indexCnt, ShouldEqual, 3)
		})
	})
}

func TestGeoHaystackIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()
	dbName := "geohaystack_test"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available")
	}

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 5.0 or later")
	}

	Convey("With a test MongoRestore instance", t, func() {
		testdb := session.Database(dbName)

		// Drop the collection to clean up resources
		defer testdb.Collection("foo").Drop(ctx)

		args := []string{
			DirectoryOption, "testdata/coll_with_geohaystack_index",
			DropOption,
		}

		restore, err := getRestoreWithArgs(args...)
		So(err, ShouldBeNil)
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		So(result.Err, ShouldNotBeNil)

		So(result.Err.Error(), ShouldContainSubstring, "found a geoHaystack index")
	})
}

func createTimeseries(dbName, coll string, client *mongo.Client) {
	timeseriesOptions := bson.M{
		"timeField": "ts",
		"metaField": "meta",
	}
	createCmd := bson.D{
		{"create", coll},
		{"timeseries", timeseriesOptions},
	}
	client.Database(dbName).RunCommand(context.Background(), createCmd)
}

func TestRestoreTimeseriesCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()
	dbName := "timeseries_test"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	if err != nil {
		t.Fatalf("No cluster available: %v", err)
	}

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	if err != nil {
		t.Fatalf("No client available")
	}

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 5.0 or later")
	}

	Convey("With a test MongoRestore instance", t, func() {
		testdb := session.Database(dbName)

		// Drop the collection to clean up resources
		defer testdb.Drop(ctx)

		args := []string{}
		var restore *MongoRestore

		Convey("restoring a directory should succeed", func() {
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err = getRestoreWithArgs(args...)

			So(err, ShouldBeNil)

		})

		Convey("restoring an archive should succeed", func() {
			args = append(args, ArchiveOption+"=testdata/timeseries_tests/dump.archive")
			restore, err = getRestoreWithArgs(args...)

			So(err, ShouldBeNil)
		})

		Convey("restoring an archive from stdin should succeed", func() {
			args = append(args, ArchiveOption+"=-")
			restore, err = getRestoreWithArgs(args...)

			archiveFile, err := os.Open("testdata/timeseries_tests/dump.archive")
			So(err, ShouldBeNil)
			restore.InputReader = archiveFile
		})
		defer restore.Close()

		// Run mongorestore
		result := restore.Restore()
		So(result.Err, ShouldBeNil)
		So(result.Successes, ShouldEqual, 10)
		So(result.Failures, ShouldEqual, 0)

		count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 1000)

		count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 10)
	})

	Convey("With a test MongoRestore instance", t, func() {
		testdb := session.Database(dbName)

		// Drop the collection to clean up resources
		defer testdb.Drop(ctx)

		args := []string{}

		Convey("restoring a timeseries collection that already exists on the destination should fail", func() {
			createTimeseries(dbName, "foo_ts", session)
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldNotBeNil)
		})

		Convey("restoring a timeseries collection when the system.buckets collection already exists on the destination should fail", func() {
			testdb.RunCommand(context.Background(), bson.M{"create": "system.buckets.foo_ts"})
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldNotBeNil)
		})

		Convey("restoring a timeseries collection with --oplogReplay should apply changes to the system.buckets collection correctly", func() {
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump_with_oplog", OplogReplayOption)
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 2164)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("restoring a timeseries collection that already exists on the destination with --drop should succeed", func() {
			createTimeseries(dbName, "foo_ts", session)
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump", DropOption)
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("restoring a timeseries collection with --noOptionsRestore should fail", func() {
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump", NoOptionsRestoreOption)
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldNotBeNil)
		})

		Convey("restoring a timeseries collection with invalid system.buckets should fail validation", func() {
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump_invalid_buckets")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 5)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("restoring a timeseries collection with invalid system.buckets should fail validation even with --bypassDocumentValidation", func() {
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump_invalid_buckets", BypassDocumentValidationOption)
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 5)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("timeseries collection should be restored if the system.buckets BSON file is used and the metadata exists", func() {
			args = append(args, DBOption, dbName, CollectionOption, "foo_ts", "testdata/timeseries_tests/ts_dump/timeseries_test/system.buckets.foo_ts.bson")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("timeseries collection should be restored if the system.buckets BSON file is used and the metadata exists and it should be renamed to --collection", func() {
			args = append(args, DBOption, dbName, CollectionOption, "bar_ts", "testdata/timeseries_tests/ts_dump/timeseries_test/system.buckets.foo_ts.bson")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("bar_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.bar_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("restoring a single system.buckets BSON file (with no metadata) should fail", func() {
			args = append(args, DBOption, dbName, CollectionOption, "system.buckets.foo_ts", "testdata/timeseries_tests/ts_single_buckets_file/system.buckets.foo_ts.bson")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldNotBeNil)
		})

		Convey("system.buckets should be restored if the timeseries collection is included in --nsInclude", func() {
			args = append(args, NSIncludeOption, dbName+".foo_ts", DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("system.buckets should not be restored if the timeseries collection is not included in --nsInclude", func() {
			args = append(args, NSIncludeOption, dbName+".system.buckets.foo_ts", DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("system.buckets should not be restored if the timeseries collection is excluded by --nsExclude", func() {
			args = append(args, NSExcludeOption, dbName+".foo_ts", DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 0)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("--noIndexRestore should stop secondary indexes from being built but should have no impact on the clustered index of system.buckets", func() {
			args = append(args, DirectoryOption, "testdata/timeseries_tests/ts_dump", NoIndexRestoreOption)
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)

			indexes, err := testdb.Collection("foo_ts").Indexes().List(ctx)
			defer indexes.Close(ctx)
			So(err, ShouldBeNil)

			numIndexes := 0
			for indexes.Next(ctx) {
				numIndexes++
			}

			if (restore.serverVersion.GTE(db.Version{6, 3, 0})) {
				Convey("--noIndexRestore should build the index on meta, time by default for time-series collections if server version >= 6.3.0", func() {
					So(numIndexes, ShouldEqual, 1)
				})
			} else {
				So(numIndexes, ShouldEqual, 0)
			}

			cur, err := testdb.ListCollections(ctx, bson.M{"name": "system.buckets.foo_ts"})
			So(err, ShouldBeNil)

			for cur.Next(ctx) {
				optVal, err := cur.Current.LookupErr("options")
				So(err, ShouldBeNil)

				optRaw, ok := optVal.DocumentOK()
				So(ok, ShouldBeTrue)

				clusteredIdxVal, err := optRaw.LookupErr("clusteredIndex")
				So(err, ShouldBeNil)

				clusteredIdx := clusteredIdxVal.Boolean()
				So(clusteredIdx, ShouldBeTrue)
			}
		})

		Convey("system.buckets should be renamed if the timeseries collection is renamed", func() {
			args = append(args, NSFromOption, dbName+".foo_ts", NSToOption, dbName+".foo_rename_ts", DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("foo_rename_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.foo_rename_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)
		})

		Convey("system.buckets collection should not be renamed if the timeseries collection is not renamed, even if the user tries to rename the system.buckets collection", func() {
			args = append(args, NSFromOption, dbName+".system.buckets.foo_ts", NSToOption, dbName+".system.buckets.foo_rename_ts", DirectoryOption, "testdata/timeseries_tests/ts_dump")
			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)

			result := restore.Restore()
			defer restore.Close()
			So(result.Err, ShouldBeNil)
			So(result.Successes, ShouldEqual, 10)
			So(result.Failures, ShouldEqual, 0)

			count, err := testdb.Collection("foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1000)

			count, err = testdb.Collection("system.buckets.foo_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 10)

			count, err = testdb.Collection("foo_rename_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)

			count, err = testdb.Collection("system.buckets.foo_rename_ts").CountDocuments(nil, bson.M{})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// ----------------------------------------------------------------------
// All tests from this point onwards use testify, not convey. See the
// CONTRIBUING.md file in the top level of the repo for details on how to
// write tests using testify.
// ----------------------------------------------------------------------

type indexInfo struct {
	name string
	keys []string
	// columnstoreProjection contains info about columnstoreProjection key in columnstore indexes.
	columnstoreProjection map[string]int32
}

func TestRestoreClusteredIndex(t *testing.T) {
	require := require.New(t)

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.3"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 5.3 or later and we have %s", fcv)
	}

	t.Run("restore from dump with default index name", func(t *testing.T) {
		testRestoreClusteredIndexFromDump(t, "")
	})
	t.Run("restore from dump with custom index name", func(t *testing.T) {
		testRestoreClusteredIndexFromDump(t, "custom index name")
	})

	res := session.Database("admin").RunCommand(context.Background(), bson.M{"replSetGetStatus": 1})
	if res.Err() != nil {
		t.Skip("server is not part of a replicaset so we cannot test restore from oplog")
	}
	t.Run("restore from oplog with default index name", func(t *testing.T) {
		testRestoreClusteredIndexFromOplog(t, "")
	})
	t.Run("restore from oplog with default index name", func(t *testing.T) {
		testRestoreClusteredIndexFromOplog(t, "custom index name")
	})
}

func testRestoreClusteredIndexFromDump(t *testing.T, indexName string) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	defer func() {
		err = testDB.Drop(nil)
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	dataLen := createClusteredIndex(t, testDB, indexName)

	withMongodump(t, testDB.Name(), "stocks", func(dir string) {
		restore, err := getRestoreWithArgs(
			DropOption,
			dir,
		)
		require.NoError(err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(result.Err, "can run mongorestore")
		require.EqualValues(dataLen, result.Successes, "mongorestore reports %d successes", dataLen)
		require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		assertClusteredIndex(t, testDB, indexName)
	})
}

func testRestoreClusteredIndexFromOplog(t *testing.T, indexName string) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	defer func() {
		err = testDB.Drop(nil)
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	createClusteredIndex(t, testDB, indexName)

	withOplogMongoDump(t, dbName, "stocks", func(dir string) {
		restore, err := getRestoreWithArgs(
			DropOption,
			OplogReplayOption,
			dir,
		)
		require.NoError(err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(result.Err, "can run mongorestore")
		require.EqualValues(0, result.Successes, "mongorestore reports 0 successes")
		require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		assertClusteredIndex(t, testDB, indexName)
	})
}

func createClusteredIndex(t *testing.T, testDB *mongo.Database, indexName string) int {
	require := require.New(t)

	fmt.Printf("creating index in %s db\n", testDB.Name())
	indexOpts := bson.M{
		"key":    bson.M{"_id": 1},
		"unique": true,
	}
	if indexName != "" {
		indexOpts["name"] = indexName
	}
	createCollCmd := bson.D{
		{Key: "create", Value: "stocks"},
		{Key: "clusteredIndex", Value: indexOpts},
	}
	res := testDB.RunCommand(context.Background(), createCollCmd, nil)
	require.NoError(res.Err(), "can create a clustered collection")

	var r interface{}
	err := res.Decode(&r)
	require.NoError(err)

	stocks := testDB.Collection("stocks")
	stockData := []interface{}{
		bson.M{"ticker": "MDB", "price": 245.33},
		bson.M{"ticker": "GOOG", "price": 2214.91},
		bson.M{"ticker": "BLZE", "price": 6.23},
	}
	_, err = stocks.InsertMany(context.Background(), stockData)
	require.NoError(err, "can insert documents into collection")

	return len(stockData)
}

func assertClusteredIndex(t *testing.T, testDB *mongo.Database, indexName string) {
	require := require.New(t)

	c, err := testDB.ListCollections(context.Background(), bson.M{})
	require.NoError(err, "can get list of collections")

	type collectionRes struct {
		Name    string
		Type    string
		Options bson.M
		Info    bson.D
		IdIndex bson.D
	}

	var collections []collectionRes
	// two Indexes should be created in addition to the _id, foo and foo_2
	for c.Next(context.Background()) {
		var res collectionRes
		err = c.Decode(&res)
		require.NoError(err, "can decode collection result")
		collections = append(collections, res)
	}

	require.Len(collections, 1, "database has one collection")
	require.Equal("stocks", collections[0].Name, "collection is named stocks")
	idx := clusteredIndexInfo(t, collections[0].Options)
	expectName := indexName
	if expectName == "" {
		expectName = "_id_"
	}
	require.Equal(expectName, idx.name, "index is named '%s'", expectName)
	require.Equal([]string{"_id"}, idx.keys, "index key is the '_id' field")
}

func clusteredIndexInfo(t *testing.T, options bson.M) indexInfo {
	idx, found := options["clusteredIndex"]
	require.True(t, found, "options has key named 'clusteredIndex'")
	require.IsType(t, bson.M{}, idx, "idx value is a bson.M")

	idxM := idx.(bson.M)
	name, found := idxM["name"]
	require.True(t, found, "index has a key named 'name'")
	require.IsType(t, "string", name, "key value is a string")

	keys, found := idxM["key"]
	require.True(t, found, "index has a key named 'key'")
	require.IsType(t, bson.M{}, keys, "key value is a bson.M")

	keysM := keys.(bson.M)
	var keyNames []string
	for k := range keysM {
		keyNames = append(keyNames, k)
	}

	return indexInfo{
		name: name.(string),
		keys: keyNames,
	}
}

func withMongodump(t *testing.T, db string, collection string, testCase func(string)) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	runMongodump(t, dir, db, collection)
	testCase(dir)
}

func withOplogMongoDump(t *testing.T, db string, collection string, testCase func(string)) {
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	// This queries the local.oplog.rs collection for commands or CRUD
	// operations on the collection we are testing (which will have a unique
	// name for each test).
	query := map[string]interface{}{
		"$or": []map[string]string{
			{"ns": fmt.Sprintf("%s.$cmd", db)},
			{"ns": fmt.Sprintf("%s.%s", db, collection)},
		},
	}
	q, err := json.Marshal(query)
	require.NoError(err, "can marshal query to JSON")

	// We dump just the documents matching the query using mongodump "normally".
	bsonFile := runMongodump(t, dir, "local", "oplog.rs", "--query", string(q))

	// Then we take the BSON dump file and rename it to "oplog.bson" and put
	// it in the root of the dump directory.
	newPath := filepath.Join(dir, "oplog.bson")
	err = os.Rename(bsonFile, newPath)
	require.NoError(err, "can rename %s -> %s", bsonFile, newPath)

	// Finally, we remove the "local" dir created by mongodump so that
	// mongorestore doesn't see it.
	localDir := filepath.Join(dir, "local")
	err = os.RemoveAll(localDir)
	require.NoError(err, "can remove %s", localDir)

	// With all that done, we now have a tree on disk like this:
	//
	// /tmp/mongorestore_test1152384390
	// └── oplog.bson
	//
	// We can run `mongorestore --oplogReplay /tmp/mongorestore_test1152384390`
	// to do a restore from the oplog.bson file.

	testCase(dir)
}

func runMongodump(t *testing.T, dir, db, collection string, args ...string) string {
	require := require.New(t)

	cmd := []string{"go", "run", filepath.Join("..", "mongodump", "main")}
	cmd = append(cmd, testutil.GetBareArgs()...)
	cmd = append(
		cmd,
		"--out", dir,
		"--db", db,
		"--collection", collection,
	)
	cmd = append(cmd, args...)
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	cmdStr := strings.Join(cmd, " ")
	require.NoError(err, "can execute command %s with output: %s", cmdStr, out)
	require.NotContains(
		string(out),
		"does not exist",
		"running [%s] does not tell us the the namespace does not exist",
		cmdStr,
	)

	bsonFile := filepath.Join(dir, db, fmt.Sprintf("%s.bson", collection))
	_, err = os.Stat(bsonFile)
	require.NoError(err, "dump created BSON data file")
	_, err = os.Stat(filepath.Join(dir, db, fmt.Sprintf("%s.metadata.json", collection)))
	require.NoError(err, "dump created JSON metadata file")

	return bsonFile
}

func uniqueDBName() string {
	return fmt.Sprintf("mongorestore_test_%d_%d", os.Getpid(), time.Now().UnixMilli())
}

// TestRestoreColumnstoreIndex tests restoring Columnstore Indexes in restored collections.
func TestRestoreColumnstoreIndex(t *testing.T) {
	require := require.New(t)

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "6.3"); err != nil || cmp < 0 {
		t.Skipf("Requires server with FCV 6.3 or later and we have %s", fcv)
	}

	t.Run("restore from dump", func(t *testing.T) {
		testRestoreColumnstoreIndexFromDump(t)
	})

	res := session.Database("admin").RunCommand(context.Background(), bson.M{"replSetGetStatus": 1})
	if res.Err() != nil {
		t.Skip("server is not part of a replicaset so we cannot test restore from oplog")
	}
	t.Run("restore from oplog", func(t *testing.T) {
		testRestoreColumnstoreIndexFromOplog(t)
	})
}

// testRestoreColumnstoreIndexFromDump tests restoring Columnstore Indexes from dump files.
func testRestoreColumnstoreIndexFromDump(t *testing.T) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	defer func() {
		err = testDB.Drop(nil)
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	key := "$**"
	columnstoreProjection := map[string]int32{"price": 1}
	dataLen := createColumnstoreIndex(t, testDB, key, columnstoreProjection)

	withMongodump(t, testDB.Name(), "stocks", func(dir string) {
		restore, err := getRestoreWithArgs(
			DropOption,
			dir,
		)
		require.NoError(err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(result.Err, "can run mongorestore")
		require.EqualValues(dataLen, result.Successes, "mongorestore reports %d successes", dataLen)
		require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		assertColumnstoreIndex(t, testDB, key, columnstoreProjection)
	})
}

// createColumnstoreIndex creates a collection with a Columnstore Index in testDB.
// The created Columnstore Index has key and columnstoreProjection specified in the function argument.
func createColumnstoreIndex(t *testing.T, testDB *mongo.Database, key string, columnstoreProjection map[string]int32) int {
	require := require.New(t)

	createCollCmd := bson.D{
		{Key: "create", Value: "stocks"},
	}
	res := testDB.RunCommand(context.Background(), createCollCmd, nil)
	require.NoError(res.Err(), "can create a collection")

	fmt.Printf("creating index in %s db\n", testDB.Name())
	indexOpts := bson.M{
		"key":                   bson.D{{key, "columnstore"}},
		"columnstoreProjection": columnstoreProjection,
		"name":                  "columnstore_index_test",
	}

	createIndexCmd := bson.D{
		{"createIndexes", "stocks"},
		{"indexes", bson.A{
			indexOpts,
		}},
	}
	res = testDB.RunCommand(context.Background(), createIndexCmd, nil)
	if strings.Contains(res.Err().Error(), "(NotImplemented) columnstore indexes are under development and cannot be used without enabling the feature flag") {
		t.Skip("Requires columnstore indexes to be implemented")
	}

	require.NoError(res.Err(), "can create a columnstore index")

	var r interface{}
	err := res.Decode(&r)
	require.NoError(err)

	stocks := testDB.Collection("stocks")
	stockData := []interface{}{
		bson.M{"ticker": "MDB", "price": 245.33},
		bson.M{"ticker": "GOOG", "price": 2214.91},
		bson.M{"ticker": "BLZE", "price": 6.23},
	}
	_, err = stocks.InsertMany(context.Background(), stockData)
	require.NoError(err, "can insert documents into collection")

	return len(stockData)
}

// assertColumnstoreIndex asserts the "stock" collection in testDB has a Columnstore Index with the expected key and the expected columnstoreProjection field.
func assertColumnstoreIndex(t *testing.T, testDB *mongo.Database, expectedKey string, expectedColumnstoreProjection map[string]int32) {
	require := require.New(t)

	c, err := testDB.ListCollections(context.Background(), bson.M{})
	require.NoError(err, "can get list of collections")

	type collectionRes struct {
		Name    string
		Type    string
		Options bson.M
		Info    bson.D
		IdIndex bson.D
	}

	var collections []collectionRes

	for c.Next(context.Background()) {
		var res collectionRes
		err = c.Decode(&res)
		require.NoError(err, "can decode collection result")
		collections = append(collections, res)
	}

	require.Len(collections, 1, "database has one collection")
	require.Equal("stocks", collections[0].Name, "collection is named stocks")
	idx := columnstoreIndexInfo(t, testDB.Collection(collections[0].Name))
	require.Equal(expectedKey, idx.keys[0], "columnstore index key is the expected key")
	require.Equal(expectedColumnstoreProjection, idx.columnstoreProjection, "columnstoreProjection is expected")
}

// columnstoreIndexInfo collects info about the Columnstore Index from the test collection.
// columnstoreIndexInfo returns non-empty indexInfo with the name, keys, and columnstoreProjection of a Columnstore Index if present in the collection.
func columnstoreIndexInfo(t *testing.T, collection *mongo.Collection) indexInfo {
	c, err := collection.Indexes().List(context.Background())
	require.NoError(t, err, "can list indexes")

	type indexRes struct {
		Name                  string
		Key                   bson.D
		ColumnstoreProjection bson.M
	}

	var columnstoreIndexInfo *indexInfo
	for c.Next(context.Background()) {
		isColumnstore := false
		var res indexRes
		err = c.Decode(&res)
		require.NoError(t, err, "can decode index")
		require.Equal(t, 1, len(res.Key), "has one index key")

		// Find the key "columnstore".
		for _, keyVal := range res.Key {
			if keyVal.Value == "columnstore" {
				isColumnstore = true
			}
		}

		if isColumnstore {
			var keyNames []string
			for _, keyVal := range res.Key {
				keyNames = append(keyNames, keyVal.Key)
			}

			columnstoreProjectionMap := map[string]int32{}
			for key, val := range res.ColumnstoreProjection {
				columnstoreProjectionMap[key] = val.(int32)
			}

			columnstoreIndexInfo = &indexInfo{
				name:                  res.Name,
				keys:                  keyNames,
				columnstoreProjection: columnstoreProjectionMap,
			}
		}
	}
	require.NotNil(t, columnstoreIndexInfo, "found columnstore index info")
	return *columnstoreIndexInfo
}

// testRestoreColumnstoreIndexFromDump tests restoring Columnstore Indexes from oplog replay.
func testRestoreColumnstoreIndexFromOplog(t *testing.T) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	defer func() {
		err = testDB.Drop(nil)
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	key := "$**"
	columnstoreProjection := map[string]int32{"price": 1}
	createColumnstoreIndex(t, testDB, key, columnstoreProjection)

	withOplogMongoDump(t, dbName, "stocks", func(dir string) {
		restore, err := getRestoreWithArgs(
			DropOption,
			OplogReplayOption,
			dir,
		)
		require.NoError(err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(result.Err, "can run mongorestore")
		require.EqualValues(0, result.Successes, "mongorestore reports 0 successes")
		require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		assertColumnstoreIndex(t, testDB, key, columnstoreProjection)
	})
}
