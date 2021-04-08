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
	"os"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
				t.Errorf("Cannot bootstrap test harness: %v", err.Error())
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
				t.Errorf("Cannot bootstrap test harness: %v", err.Error())
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

	Convey("PreserveUUID restore with incompatible destination FCV errors", t, func() {
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
		So(result.Err, ShouldNotBeNil)
		So(result.Err.Error(), ShouldContainSubstring, "target host does not support --preserveUUID")
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
