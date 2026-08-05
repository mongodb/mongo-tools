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

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/xoptions"
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

var testDocument = bson.M{"key": "value"}

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
	// are well-defined only for restoration of a single BSON file.
	//
	// Hacky way of looking at the application log at test-time: ideally we
	// would use some form of explicit dependency injection to specify the
	// sink for the parsing/validation log, but the validation logic here is
	// coupled with the mongorestore.MongoRestore type, which does not
	// support such an injection.

	t.Run("and no warning is issued in the well-defined case", func(t *testing.T) {
		var buffer bytes.Buffer

		log.SetWriter(&buffer)
		defer log.SetWriter(os.Stderr)

		args := []string{
			"testdata/hashedIndexes.bson",
			DBOption, "db1	",
			CollectionOption, "coll1",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should bootstrap the test harness")
		defer restore.Close()

		err = restore.ParseAndValidateOptions()

		require.NoError(t, err, "should accept --db and --collection for a single BSON file")
		assert.Empty(
			t,
			buffer.String(),
			"should not warn when --db and --collection are well-defined",
		)
	})

	t.Run("and a warning is issued in the deprecated case", func(t *testing.T) {
		var buffer bytes.Buffer

		log.SetWriter(&buffer)
		defer log.SetWriter(os.Stderr)

		args := []string{
			DBOption, "db1",
			CollectionOption, "coll1",
		}

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should bootstrap the test harness")
		defer restore.Close()

		err = restore.ParseAndValidateOptions()

		require.NoError(t, err, "should accept the deprecated --db and --collection combination")
		assert.Contains(
			t,
			buffer.String(),
			deprecatedDBAndCollectionsOptionsWarning,
			"should warn about the deprecated --db and --collection combination",
		)
	})
}

func TestMongorestore(t *testing.T) {
	testtype.RequireMatchingTestType(
		t,
		testtype.IntegrationTestType,
		testtype.ShardedIntegrationTestType,
	)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	t.Run("and majority is used as the default write concern", func(t *testing.T) {
		restore, err := getRestoreWithArgs(
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		assert.Equal(
			t,
			wcwrapper.Majority(),
			restore.ToolOptions.WriteConcern,
			"should default to majority write concern",
		)
	})

	t.Run("and an explicit target restores from that dump directory", func(t *testing.T) {
		restore, _, c1, c4 := newMongorestoreTestFixture(t, session)
		restore.TargetDirectory = "testdata/testdirs"

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 110, result.Successes, "should restore every document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := c1.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c1")
		assert.EqualValues(t, 100, count, "should restore all c1 documents")

		count, err = c4.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c4")
		assert.EqualValues(t, 10, count, "should restore all c4 documents")
	})

	t.Run("and an target of '-' restores from standard input", func(t *testing.T) {
		restore, _, c1, _ := newMongorestoreTestFixture(t, session)

		bsonFile, err := os.Open("testdata/testdirs/db1/c1.bson")
		require.NoError(t, err, "should open the test fixture")

		restore.ToolOptions.Collection = "c1"
		restore.ToolOptions.DB = "db1"
		restore.InputReader = bsonFile
		restore.TargetDirectory = "-"

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		count, err := c1.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c1")
		assert.EqualValues(t, 100, count, "should restore all c1 documents from standard input")
	})

	t.Run("and specifying an nsExclude option", func(t *testing.T) {
		restore, _, c1, c4 := newMongorestoreTestFixture(t, session)
		restore.TargetDirectory = "testdata/testdirs"
		restore.NSOptions.NSExclude = make([]string, 1)
		restore.NSOptions.NSExclude[0] = "db1.c1"

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 10, result.Successes, "should restore only the included documents")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := c1.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the documents in the excluded namespace c1")
		assert.EqualValues(t, 0, count, "should not restore documents in the excluded namespace")

		count, err = c4.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c4")
		assert.EqualValues(t, 10, count, "should restore all c4 documents")
	})

	t.Run("and specifying an nsInclude option", func(t *testing.T) {
		restore, _, c1, c4 := newMongorestoreTestFixture(t, session)
		restore.TargetDirectory = "testdata/testdirs"
		restore.NSOptions.NSInclude = make([]string, 1)
		restore.NSOptions.NSInclude[0] = "db1.c4"

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 10, result.Successes, "should restore only the included documents")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := c1.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the documents in the excluded namespace c1")
		assert.EqualValues(
			t,
			0,
			count,
			"should not restore documents outside the included namespace",
		)

		count, err = c4.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c4")
		assert.EqualValues(t, 10, count, "should restore all c4 documents")
	})

	t.Run("and specifying nsFrom and nsTo options", func(t *testing.T) {
		restore, db, _, c4 := newMongorestoreTestFixture(t, session)
		restore.TargetDirectory = "testdata/testdirs"

		restore.NSOptions.NSFrom = make([]string, 1)
		restore.NSOptions.NSFrom[0] = "db1.c1"
		restore.NSOptions.NSTo = make([]string, 1)
		restore.NSOptions.NSTo[0] = "db1.c1renamed"

		c1renamed := db.Collection("c1renamed")
		require.NoError(t, c1renamed.Drop(t.Context()), "should drop c1renamed before restoring")

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 110, result.Successes, "should restore every document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := c1renamed.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c1renamed")
		assert.EqualValues(
			t,
			100,
			count,
			"should restore all c1 documents under the renamed namespace",
		)

		count, err = c4.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents in c4")
		assert.EqualValues(t, 10, count, "should restore all c4 documents")
	})
}

// newMongorestoreTestFixture builds a fresh MongoRestore instance and drops
// the shared test collections, so each subtest starts from clean, isolated
// state rather than sharing a restore instance whose TargetDirectory and
// NSOptions the other subtests mutate. Cleanup is registered per subtest.
func newMongorestoreTestFixture(
	t *testing.T,
	session *mongo.Client,
) (restore *MongoRestore, db *mongo.Database, c1, c4 *mongo.Collection) {
	t.Helper()

	restore, err := getRestoreWithArgs(
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
	)
	require.NoError(t, err, "should build a restore instance")
	t.Cleanup(restore.Close)

	db = session.Database("db1")

	c1 = db.Collection("c1") // 100 documents
	require.NoError(t, c1.Drop(t.Context()), "should drop c1 before restoring")
	c2 := db.Collection("c2") // 0 documents
	require.NoError(t, c2.Drop(t.Context()), "should drop c2 before restoring")
	c3 := db.Collection("c3") // 0 documents
	require.NoError(t, c3.Drop(t.Context()), "should drop c3 before restoring")
	c4 = db.Collection("c4") // 10 documents
	require.NoError(t, c4.Drop(t.Context()), "should drop c4 before restoring")

	return restore, db, c1, c4
}

func TestMongoRestoreSpecialCharactersCollectionNames(t *testing.T) {
	testtype.RequireMatchingTestType(
		t,
		testtype.IntegrationTestType,
		testtype.ShardedIntegrationTestType,
	)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	t.Run("and --nsInclude a collection name with special characters", func(t *testing.T) {
		restore, _, specialCharacterCollection := newSpecialCharactersTestFixture(t, session)
		restore.TargetDirectory = "testdata/specialcharacter"
		restore.NSOptions.NSInclude = make([]string, 1)
		restore.NSOptions.NSInclude[0] = "db1." + specialCharactersCollectionName

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 1, result.Successes, "should restore the included document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := specialCharacterCollection.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(
			t,
			1,
			count,
			"should restore the document into the special-character collection",
		)
	})

	t.Run("and --nsExclude a collection name with special characters", func(t *testing.T) {
		restore, _, specialCharacterCollection := newSpecialCharactersTestFixture(t, session)
		restore.TargetDirectory = "testdata/specialcharacter"
		restore.NSOptions.NSExclude = make([]string, 1)
		restore.NSOptions.NSExclude[0] = "db1." + specialCharactersCollectionName

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(
			t,
			0,
			result.Successes,
			"should restore nothing when the only namespace is excluded",
		)
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := specialCharacterCollection.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the documents in the excluded collection")
		assert.EqualValues(t, 0, count, "should not restore documents into the excluded collection")
	})

	t.Run("and --nsTo a collection name without special characters "+
		"--nsFrom a collection name with special characters", func(t *testing.T) {
		restore, db, _ := newSpecialCharactersTestFixture(t, session)
		restore.TargetDirectory = "testdata/specialcharacter"
		restore.NSOptions.NSFrom = make([]string, 1)
		restore.NSOptions.NSFrom[0] = "db1." + specialCharactersCollectionName
		restore.NSOptions.NSTo = make([]string, 1)
		restore.NSOptions.NSTo[0] = "db1.aCollectionNameWithoutSpecialCharacters"

		standardCharactersCollection := db.Collection("aCollectionNameWithoutSpecialCharacters")
		require.NoError(
			t,
			standardCharactersCollection.Drop(t.Context()),
			"should drop the destination collection before restoring",
		)

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 1, result.Successes, "should restore the renamed document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := standardCharactersCollection.CountDocuments(
			t.Context(),
			bson.M{},
		)
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(t, 1, count, "should restore the document under the renamed namespace")
	})

	t.Run("and --nsTo a collection name with special characters "+
		"--nsFrom a collection name with special characters", func(t *testing.T) {
		restore, db, _ := newSpecialCharactersTestFixture(t, session)
		restore.TargetDirectory = "testdata/specialcharacter"
		restore.NSOptions.NSFrom = make([]string, 1)
		restore.NSOptions.NSFrom[0] = "db1." + specialCharactersCollectionName
		restore.NSOptions.NSTo = make([]string, 1)
		restore.NSOptions.NSTo[0] = "db1.aCollectionNameWithSpećiálCharacters"

		standardCharactersCollection := db.Collection("aCollectionNameWithSpećiálCharacters")
		require.NoError(
			t,
			standardCharactersCollection.Drop(t.Context()),
			"should drop the destination collection before restoring",
		)

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 1, result.Successes, "should restore the renamed document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := standardCharactersCollection.CountDocuments(
			t.Context(),
			bson.M{},
		)
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(t, 1, count, "should restore the document under the renamed namespace")
	})
}

// newSpecialCharactersTestFixture builds a fresh MongoRestore instance and
// drops the special-character test collection, so each subtest starts from
// clean, isolated state rather than sharing a restore instance whose
// TargetDirectory and NSOptions the other subtests mutate. Cleanup is
// registered per subtest.
func newSpecialCharactersTestFixture(
	t *testing.T,
	session *mongo.Client,
) (restore *MongoRestore, db *mongo.Database, specialCharacterCollection *mongo.Collection) {
	t.Helper()

	restore, err := getRestoreWithArgs(
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
	)
	require.NoError(t, err, "should build a restore instance")
	t.Cleanup(restore.Close)

	db = session.Database("db1")

	specialCharacterCollection = db.Collection(specialCharactersCollectionName)
	require.NoError(
		t,
		specialCharacterCollection.Drop(t.Context()),
		"should drop the special-character collection before restoring",
	)

	return restore, db, specialCharacterCollection
}

func TestMongorestoreLongCollectionName(t *testing.T) {
	// Disabled: see TOOLS-2658
	t.Skip()

	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 4.4 or later")
	}

	t.Run("and majority is used as the default write concern", func(t *testing.T) {
		restore, err := getRestoreWithArgs(
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		assert.Equal(
			t,
			wcwrapper.Majority(),
			restore.ToolOptions.WriteConcern,
			"should default to majority write concern",
		)
	})

	t.Run(
		"and an explicit target restores truncated files from that dump directory",
		func(t *testing.T) {
			restore, _, longCollection := newLongCollectionNameTestFixture(t, session)
			restore.TargetDirectory = "testdata/longcollectionname"

			result := restore.Restore()
			require.NoError(t, result.Err, "should restore without error")
			assert.EqualValues(t, 1, result.Successes, "should restore the document")
			assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

			count, err := longCollection.CountDocuments(t.Context(), bson.M{})
			require.NoError(t, err, "should count the restored documents")
			assert.EqualValues(
				t,
				1,
				count,
				"should restore the document into the long-named collection",
			)
		},
	)

	t.Run("and an target of '-' restores truncated files from standard input", func(t *testing.T) {
		restore, _, longCollection := newLongCollectionNameTestFixture(t, session)

		longBsonFile, err := os.Open("testdata/longcollectionname/db1/" + longBsonName)
		require.NoError(t, err, "should open the test fixture")

		restore.ToolOptions.Collection = longCollectionName
		restore.ToolOptions.DB = "db1"
		restore.InputReader = longBsonFile
		restore.TargetDirectory = "-"
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")

		count, err := longCollection.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(t, 1, count, "should restore the document from standard input")
	})

	t.Run("and specifying an nsExclude option", func(t *testing.T) {
		restore, _, longCollection := newLongCollectionNameTestFixture(t, session)
		restore.TargetDirectory = "testdata/longcollectionname"
		restore.NSOptions.NSExclude = make([]string, 1)
		restore.NSOptions.NSExclude[0] = "db1." + longCollectionName

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(
			t,
			0,
			result.Successes,
			"should restore nothing when the only namespace is excluded",
		)
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := longCollection.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the documents in the excluded collection")
		assert.EqualValues(t, 0, count, "should not restore documents into the excluded collection")
	})

	t.Run("and specifying an nsInclude option", func(t *testing.T) {
		restore, _, longCollection := newLongCollectionNameTestFixture(t, session)
		restore.TargetDirectory = "testdata/longcollectionname"
		restore.NSOptions.NSInclude = make([]string, 1)
		restore.NSOptions.NSInclude[0] = "db1." + longCollectionName

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 1, result.Successes, "should restore the included document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := longCollection.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(
			t,
			1,
			count,
			"should restore the document into the long-named collection",
		)
	})

	t.Run("and specifying nsFrom and nsTo options", func(t *testing.T) {
		restore, db, _ := newLongCollectionNameTestFixture(t, session)
		restore.TargetDirectory = "testdata/longcollectionname"
		restore.NSOptions.NSFrom = make([]string, 1)
		restore.NSOptions.NSFrom[0] = "db1." + longCollectionName
		restore.NSOptions.NSTo = make([]string, 1)
		restore.NSOptions.NSTo[0] = "db1.aMuchShorterCollectionName"

		shortCollection := db.Collection("aMuchShorterCollectionName")
		require.NoError(
			t,
			shortCollection.Drop(t.Context()),
			"should drop the destination collection before restoring",
		)

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		assert.EqualValues(t, 1, result.Successes, "should restore the renamed document")
		assert.EqualValues(t, 0, result.Failures, "should not fail to restore any document")

		count, err := shortCollection.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(t, 1, count, "should restore the document under the renamed namespace")
	})
}

// newLongCollectionNameTestFixture builds a fresh MongoRestore instance and
// drops the long-named test collection, so each subtest starts from clean,
// isolated state rather than sharing a restore instance whose
// TargetDirectory and NSOptions the other subtests mutate. Cleanup is
// registered per subtest.
func newLongCollectionNameTestFixture(
	t *testing.T,
	session *mongo.Client,
) (restore *MongoRestore, db *mongo.Database, longCollection *mongo.Collection) {
	t.Helper()

	restore, err := getRestoreWithArgs(
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
	)
	require.NoError(t, err, "should build a restore instance")
	t.Cleanup(restore.Close)

	db = session.Database("db1")

	longCollection = db.Collection(longCollectionName)
	require.NoError(
		t,
		longCollection.Drop(t.Context()),
		"should drop the long-named collection before restoring",
	)

	return restore, db, longCollection
}

func TestMongorestorePreserveUUID(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "3.6"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 3.6 or later")
	}

	// From mongorestore/testdata/oplogdump/db1/c1.metadata.json
	originalUUID := "699f503df64b4aa8a484a8052046fa3a"

	t.Run("normal restore gives new UUID", func(t *testing.T) {
		c1 := session.Database("db1").Collection("c1")
		require.NoError(t, c1.Drop(t.Context()), "should drop c1 before restoring")

		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			"testdata/oplogdump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		count, err := c1.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(t, 5, count, "should restore every document")
		info, err := db.GetCollectionInfo(c1)
		require.NoError(t, err, "should read the restored collection info")
		assert.NotEqual(
			t,
			originalUUID,
			info.GetUUID(),
			"should assign a new UUID without --preserveUUID",
		)
	})

	t.Run("PreserveUUID restore without drop errors", func(t *testing.T) {
		c1 := session.Database("db1").Collection("c1")
		require.NoError(t, c1.Drop(t.Context()), "should drop c1 before restoring")

		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			PreserveUUIDOption,
			"testdata/oplogdump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		result := restore.Restore()
		require.Error(t, result.Err, "should reject --preserveUUID without --drop")
		assert.Contains(
			t,
			result.Err.Error(),
			"cannot specify --preserveUUID without --drop",
			"should explain that --preserveUUID requires --drop",
		)
	})

	t.Run("PreserveUUID with drop preserves UUID", func(t *testing.T) {
		c1 := session.Database("db1").Collection("c1")
		require.NoError(t, c1.Drop(t.Context()), "should drop c1 before restoring")

		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			PreserveUUIDOption,
			DropOption,
			"testdata/oplogdump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
		count, err := c1.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		assert.EqualValues(t, 5, count, "should restore every document")
		info, err := db.GetCollectionInfo(c1)
		require.NoError(t, err, "should read the restored collection info")
		assert.Equal(
			t,
			originalUUID,
			info.GetUUID(),
			"should preserve the original UUID with --preserveUUID and --drop",
		)
	})

	t.Run("PreserveUUID on a file without UUID metadata errors", func(t *testing.T) {
		c1 := session.Database("db1").Collection("c1")
		require.NoError(t, c1.Drop(t.Context()), "should drop c1 before restoring")

		args := []string{
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
			PreserveUUIDOption,
			DropOption,
			"testdata/testdirs",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")
	})
}

// generateTestData creates the files used in TestMongorestoreMIOSOE.
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

// test --maintainInsertionOrder and --stopOnError behavior.
func TestMongorestoreMIOSOE(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	require.NoError(t, generateTestData(), "should generate the test data")

	client, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	database := client.Database("miodb")
	coll := database.Collection("mio")

	t.Run("default restore ignores dup key errors", func(t *testing.T) {
		restore, err := getRestoreWithArgs(mioSoeFile,
			CollectionOption, coll.Name(),
			DBOption, database.Name(),
			DropOption)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()
		require.False(
			t,
			restore.OutputOptions.MaintainInsertionOrder,
			"should not maintain insertion order by default",
		)

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore despite duplicate key errors")
		require.EqualValues(
			t,
			20000,
			result.Successes,
			"should insert every non-duplicate document",
		)
		require.EqualValues(t, 1, result.Failures, "should count the single duplicate key error")

		count, err := coll.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(t, 20000, count, "should restore every non-duplicate document")
	})

	t.Run("--maintainInsertionOrder stops exactly on dup key errors", func(t *testing.T) {
		restore, err := getRestoreWithArgs(mioSoeFile,
			CollectionOption, coll.Name(),
			DBOption, database.Name(),
			DropOption,
			MaintainInsertionOrderOption)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()
		require.True(
			t,
			restore.OutputOptions.MaintainInsertionOrder,
			"should maintain insertion order",
		)
		require.EqualValues(
			t,
			1,
			restore.OutputOptions.NumInsertionWorkers,
			"should use a single insertion worker to maintain order",
		)

		result := restore.Restore()
		require.Error(t, result.Err, "should stop on the duplicate key error")
		require.EqualValues(
			t,
			10000,
			result.Successes,
			"should insert only documents before the duplicate",
		)
		require.EqualValues(t, 1, result.Failures, "should count the duplicate key error")

		count, err := coll.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(t, 10000, count, "should restore only documents before the duplicate")
	})

	t.Run("--stopOnError stops on dup key errors", func(t *testing.T) {
		restore, err := getRestoreWithArgs(mioSoeFile,
			CollectionOption, coll.Name(),
			DBOption, database.Name(),
			DropOption,
			StopOnErrorOption,
			NumParallelCollectionsOption, "1")
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()
		require.True(t, restore.OutputOptions.StopOnError, "should enable stop-on-error")

		result := restore.Restore()
		require.Error(t, result.Err, "should stop on the duplicate key error")
		require.InDelta(
			t,
			10000,
			result.Successes,
			float64(restore.OutputOptions.BulkBufferSize),
			"should insert approximately the documents before the duplicate",
		)
		require.EqualValues(t, 1, result.Failures, "should count the duplicate key error")

		count, err := coll.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.InDelta(
			t,
			10000,
			count,
			float64(restore.OutputOptions.BulkBufferSize),
			"should restore approximately the documents before the duplicate",
		)
	})

	err = database.Drop(t.Context())
	require.NoError(t, err, "should drop the test database")
}

func TestDeprecatedIndexOptions(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	t.Run("creating index with invalid option should throw error", func(t *testing.T) {
		restore, coll := newDeprecatedIndexOptionsRestore(
			t,
			session,
			NumParallelCollectionsOption, "1",
			NumInsertionWorkersOption, "1",
		)

		restore.TargetDirectory = "testdata/indextestdump"
		result := restore.Restore()
		require.Error(t, result.Err, "should fail to create an index with an invalid option")
		require.True(
			t,
			strings.HasPrefix(
				result.Err.Error(),
				`indextest.test_collection: error creating indexes for indextest.test_collection: createIndex error:`,
			),
			"should report the createIndex error",
		)

		require.EqualValues(
			t,
			100,
			result.Successes,
			"should insert every document before the index failure",
		)
		require.EqualValues(t, 0, result.Failures, "should not count document-insertion failures")
		count, err := coll.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err, "should count the restored documents")
		require.EqualValues(
			t,
			100,
			count,
			"should restore every document despite the index failure",
		)
	})

	t.Run(
		"creating index with invalid option and --convertLegacyIndexes should succeed",
		func(t *testing.T) {
			restore, coll := newDeprecatedIndexOptionsRestore(
				t,
				session,
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
				ConvertLegacyIndexesOption, "true",
			)

			restore.TargetDirectory = "testdata/indextestdump"
			result := restore.Restore()
			require.NoError(t, result.Err, "should convert the legacy index option and succeed")

			require.EqualValues(t, 100, result.Successes, "should insert every document")
			require.EqualValues(
				t,
				0,
				result.Failures,
				"should not fail to create the converted index",
			)
			count, err := coll.CountDocuments(t.Context(), bson.M{})
			require.NoError(t, err, "should count the restored documents")
			require.EqualValues(t, 100, count, "should restore every document")
		},
	)
}

// newDeprecatedIndexOptionsRestore builds a restore instance against a freshly
// dropped indextest.test_collection. Each subtest gets its own restore and a
// freshly dropped collection, so the two scenarios can't leak index state
// into each other.
func newDeprecatedIndexOptionsRestore(
	t *testing.T,
	session *mongo.Client,
	args ...string,
) (*MongoRestore, *mongo.Collection) {
	t.Helper()

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	t.Cleanup(restore.Close)

	coll := session.Database("indextest").Collection("test_collection")
	require.NoError(t, coll.Drop(t.Context()), "should drop the test collection")
	t.Cleanup(func() {
		// t.Context() is already canceled by the time cleanups run.
		assert.NoError(t, coll.Drop(context.Background()), "should drop the test collection")
	})

	return restore, coll
}

// TestFixDuplicatedLegacyIndexes restores two indexes with --convertLegacyIndexes flag, {foo: ""} and {foo: 1}
// Only one index {foo: 1} should be created.
func TestFixDuplicatedLegacyIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "3.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 3.4 or later")
	}

	restore, err := getRestoreWithArgs(ConvertLegacyIndexesOption)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	restore.TargetDirectory = "testdata/duplicate_index_key"
	result := restore.Restore()
	require.NoError(t, result.Err, "should skip the duplicate index key without failing")
	require.EqualValues(t, 0, result.Successes, "should not insert any documents")
	require.EqualValues(t, 0, result.Failures, "should not fail to insert any documents")
	require.NoError(t, err, "should build a restore instance")

	testDB := session.Database("indextest")
	defer func() {
		err = testDB.Drop(t.Context())
		require.NoError(t, err, "should drop the test database")
	}()

	c, err := testDB.Collection("duplicate_index_key").Indexes().List(t.Context())
	require.NoError(t, err, "should list the collection's indexes")

	type indexRes struct {
		Name string
		Key  bson.D
	}

	indexKeys := make(map[string]bson.D)

	// two Indexes should be created in addition to the _id, foo and foo_2
	for c.Next(t.Context()) {
		var res indexRes
		err = c.Decode(&res)
		require.NoError(t, err, "should decode each index")
		require.Len(t, res.Key, 1, "should have a single-field index key")
		indexKeys[res.Name] = res.Key
	}

	require.Len(t, indexKeys, 3, "should create exactly three indexes")

	var indexKey bson.D
	// Check that only one of foo_, foo_1, or foo_1.0 was created
	indexKeyFoo, ok := indexKeys["foo_"]
	indexKeyFoo1, ok1 := indexKeys["foo_1"]
	indexKeyFoo10, ok10 := indexKeys["foo_1.0"]

	require.True(t, ok || ok1 || ok10, "should create one of the duplicate-key index name variants")

	if ok {
		require.False(
			t,
			ok1 || ok10,
			"should create only one of the duplicate-key index name variants",
		)
		indexKey = indexKeyFoo
	}

	if ok1 {
		require.False(
			t,
			ok || ok10,
			"should create only one of the duplicate-key index name variants",
		)
		indexKey = indexKeyFoo1
	}

	if ok10 {
		require.False(
			t,
			ok || ok1,
			"should create only one of the duplicate-key index name variants",
		)
		indexKey = indexKeyFoo10
	}

	require.Len(t, indexKey, 1, "should have a single-field index key")
	require.Equal(t, "foo", indexKey[0].Key, "should index the foo field")
	require.EqualValues(t, 1, indexKey[0].Value, "should keep the ascending index")

	indexKey, ok = indexKeys["foo_2"]
	require.True(t, ok, "should create the second foo index")
	require.Len(t, indexKey, 1, "should have a single-field index key")
	require.Equal(t, "foo", indexKey[0].Key, "should index the foo field")
	require.EqualValues(t, 2, indexKey[0].Value, "should keep the descending index")
}

func TestDeprecatedIndexOptionsOn44FCV(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")
	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 4.4 or later")
	}

	args := []string{
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	session, _ = restore.SessionProvider.GetSession()

	db := session.Database("indextest")

	// 4.4 removes the 'ns' field nested under the 'index' field in metadata.json
	coll := db.Collection("test_coll_no_index_ns")
	err = coll.Drop(t.Context())
	require.NoError(t, err, "should drop the test collection")
	defer func() {
		dropErr := coll.Drop(t.Context())
		assert.NoError(t, dropErr, "should drop the test collection")
	}()

	args = []string{
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
		ConvertLegacyIndexesOption, "true",
	}

	restore, err = getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	restore.TargetDirectory = "testdata/indexmetadata"
	result := restore.Restore()
	require.NoError(t, result.Err, "should convert the legacy index and succeed on 4.4 FCV")

	require.EqualValues(t, 100, result.Successes, "should insert every document")
	require.EqualValues(t, 0, result.Failures, "should not fail to create the converted index")
	count, err := coll.CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err, "should count the restored documents")
	require.EqualValues(t, 100, count, "should restore every document")
}

func TestLongIndexName(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	args := []string{
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	session, err := restore.SessionProvider.GetSession()
	require.NoError(t, err, "should get a session from the restore instance")

	coll := session.Database("longindextest").Collection("test_collection")
	err = coll.Drop(t.Context())
	require.NoError(t, err, "should drop the test collection")
	defer func() {
		dropErr := coll.Drop(t.Context())
		assert.NoError(t, dropErr, "should drop the test collection")
	}()

	restore.TargetDirectory = "testdata/longindextestdump"
	result := restore.Restore()

	if restore.serverVersion.LT(db.Version{4, 2, 0}) {
		require.Error(
			t,
			result.Err,
			"should fail to create an index name longer than 127 bytes (<4.2)",
		)
		require.Contains(
			t,
			result.Err.Error(),
			"namespace is too long (max size is 127 bytes)",
			"should report the namespace-too-long error",
		)
	} else {
		require.NoError(t, result.Err, "should create an index name longer than 127 bytes (>=4.2)")

		indexes := session.Database("longindextest").Collection("test_collection").Indexes()
		c, err := indexes.List(t.Context())
		require.NoError(t, err, "should list the collection's indexes")

		type indexRes struct {
			Name string
		}
		var names []string
		for c.Next(t.Context()) {
			var r indexRes
			err := c.Decode(&r)
			require.NoError(t, err, "should decode each index")
			names = append(names, r.Name)
		}
		require.Len(t, names, 2, "should create exactly two indexes")
		sort.Strings(names)
		require.Equal(t, "_id_", names[0], "should keep the default _id index")
		require.Equal(
			t,
			"a_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			names[1],
			"should create the long index name",
		)
	}
}

func TestKnownCollections(t *testing.T) {
	testtype.RequireMatchingTestType(
		t,
		testtype.IntegrationTestType,
		testtype.ShardedIntegrationTestType,
	)
	_, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	args := []string{
		NumParallelCollectionsOption, "1",
		NumInsertionWorkersOption, "1",
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	session, _ := restore.SessionProvider.GetSession()
	db := session.Database("test")
	defer func() {
		dropErr := db.Collection("foo").Drop(t.Context())
		assert.NoError(t, dropErr, "should drop the test collection")
	}()

	restore.TargetDirectory = "testdata/foodump"
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore the foo collection")

	var namespaceExistsInCache bool
	if cols, ok := restore.knownCollections["test"]; ok {
		for _, collName := range cols {
			if collName == "foo" {
				namespaceExistsInCache = true
			}
		}
	}
	require.True(
		t,
		namespaceExistsInCache,
		"should record the restored collection in knownCollections",
	)
}

func TestReadPreludeMetadata(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	_, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	cases := []struct {
		name            string
		targetDirectory string
		gzip            bool
		db              string
		expectedVersion db.Version
	}{
		{
			name:            "sets serverDumpVersion from prelude.json when dump dir is target",
			targetDirectory: "testdata/prelude_test/prelude_top_level",
			expectedVersion: db.Version{7, 0, 16},
		},
		{
			name:            "sets serverDumpVersion from prelude.json.gz when gzipped dump is used",
			targetDirectory: "testdata/prelude_test/prelude_gzip/test",
			gzip:            true,
			expectedVersion: db.Version{7, 0, 16},
		},
		{
			name:            "sets serverDumpVersion from prelude.json in main dump dir when db dir is target",
			targetDirectory: "testdata/prelude_test/prelude_top_level/test",
			expectedVersion: db.Version{7, 0, 16},
		},
		{
			name:            "sets serverDumpVersion from prelude.json from the db's directory",
			targetDirectory: "testdata/prelude_test/prelude_db_target/test",
			db:              "test",
			expectedVersion: db.Version{7, 0, 16},
		},
		{
			name:            "sets serverDumpVersion from prelude.json in parent directory when file is used as target",
			targetDirectory: "testdata/prelude_test/prelude_top_level/test/foo.bson",
			expectedVersion: db.Version{7, 0, 16},
		},
		{
			name:            "does not error out when server version is unknown",
			targetDirectory: "testdata/prelude_test/server_version_unknown",
			expectedVersion: db.Version{},
		},
		{
			name:            "does not error out when prelude is not available",
			targetDirectory: "testdata/foodump",
			expectedVersion: db.Version{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				NumParallelCollectionsOption, "1",
				NumInsertionWorkersOption, "1",
			}

			restore, err := getRestoreWithArgs(args...)
			require.NoError(t, err, "should build a restore instance")
			defer restore.Close()

			restoreSession, _ := restore.SessionProvider.GetSession()
			defer func() {
				dropErr := restoreSession.Database("test").Collection("foo").Drop(t.Context())
				assert.NoError(t, dropErr, "should drop the test collection")
			}()

			restore.TargetDirectory = tc.targetDirectory
			restore.InputOptions.Gzip = tc.gzip
			if tc.db != "" {
				restore.ToolOptions.DB = tc.db
			}

			result := restore.Restore()
			require.NoError(t, result.Err, "should restore without error")
			require.Equal(
				t,
				tc.expectedVersion,
				restore.dumpServerVersion,
				"should read the correct server version from the prelude",
			)
		})
	}
}

func TestFixHashedIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	t.Run("with --fixHashedIndexes", func(t *testing.T) {
		restore := restoreForFixHashedIndexes(t, session, FixDottedHashedIndexesOption)

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")

		// the a.b key is dotted, so --fixHashedIndexes should change it from
		// hashed to 1; a.a and b are not dotted and keep their original values.
		assertHashedIndexKeys(
			t,
			session.Database("testdata").Collection("hashedIndexes"),
			map[string]any{
				"b":   "hashed",
				"a.a": 1,
				"a.b": 1,
			},
		)
	})

	t.Run("without --fixHashedIndexes", func(t *testing.T) {
		restore := restoreForFixHashedIndexes(t, session)

		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")

		assertHashedIndexKeys(
			t,
			session.Database("testdata").Collection("hashedIndexes"),
			map[string]any{
				"b":   "hashed",
				"a.a": 1,
				"a.b": "hashed",
			},
		)
	})
}

// restoreForFixHashedIndexes builds a restore instance targeting the shared
// hashedIndexes fixture and registers its own teardown, so each subtest gets
// a fresh restore instance and collection.
func restoreForFixHashedIndexes(t *testing.T, session *mongo.Client, args ...string) *MongoRestore {
	t.Helper()

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	t.Cleanup(restore.Close)

	db := session.Database("testdata")
	t.Cleanup(func() {
		assert.NoError(
			t,
			db.Collection("hashedIndexes").Drop(context.Background()),
			"should drop the test collection",
		)
	})

	restore.TargetDirectory = "testdata/hashedIndexes.bson"

	return restore
}

func assertHashedIndexKeys(t *testing.T, coll *mongo.Collection, expected map[string]any) {
	t.Helper()

	type indexRes struct {
		Key bson.D
	}

	indexes := coll.Indexes()
	c, err := indexes.List(t.Context())
	require.NoError(t, err, "should list the indexes")

	for c.Next(t.Context()) {
		var res indexRes
		require.NoError(t, c.Decode(&res), "should decode each index document")
		for _, key := range res.Key {
			if key.Key == "_id" {
				continue
			}
			want, ok := expected[key.Key]
			require.True(t, ok, "should not create unexpected index key %q", key.Key)
			assert.EqualValues(
				t,
				want,
				key.Value,
				"index key %q should have value %v",
				key.Key,
				want,
			)
		}
	}
}

func TestAutoIndexIdLocalDB(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "should get the server version")
	if serverVersion.GTE(db.Version{8, 2, 0}) {
		t.Skipf(
			"createCollection no longer accepts autoIndexID as of Server version 8.2.0; testing with %s",
			serverVersion.String(),
		)
	}

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	dbName := session.Database("local")

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer dbName.Collection("test_auto_idx").Drop(ctx)

	opts, err := ParseOptions(testutil.GetBareArgs(), "", "")
	require.NoError(t, err, "should parse the options")

	// Set retryWrites to false since it is unsupported on `local` db.
	retryWrites := false
	opts.RetryWrites = &retryWrites

	restore, err := New(opts)
	require.NoError(t, err, "should build a restore instance")

	restore.TargetDirectory = "testdata/local/test_auto_idx.bson"
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")

	// Find the collection
	filter := bson.D{{"name", "test_auto_idx"}}
	cursor, err := session.Database("local").ListCollections(ctx, filter)
	require.NoError(t, err, "should list the collections")

	defer cursor.Close(ctx)

	documentExists := cursor.Next(ctx)
	require.True(t, documentExists, "should find the restored collection")

	var collInfo struct {
		Options bson.M
	}
	err = cursor.Decode(&collInfo)
	require.NoError(t, err, "should decode the collection info")

	assert.Equal(
		t,
		false,
		collInfo.Options["autoIndexId"],
		"autoIndexId should remain false on a local database",
	)
}

func TestAutoIndexIdNonLocalDB(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "should get the server version")
	if serverVersion.GTE(db.Version{8, 2, 0}) {
		t.Skipf(
			"createCollection no longer accepts autoIndexID as of Server version 8.2.0; testing with %s",
			serverVersion.String(),
		)
	}

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to the server")

	t.Run("do not set --preserveUUID", func(t *testing.T) {
		dbName := session.Database("testdata")

		// Drop the collection to clean up resources
		//
		//nolint:errcheck
		defer dbName.Collection("test_auto_idx").Drop(ctx)

		var args []string

		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err, "should build a restore instance")
		defer restore.Close()

		restore.TargetDirectory = "testdata/test_auto_idx.bson"
		result := restore.Restore()
		require.NoError(t, result.Err, "should restore without error")

		// Find the collection
		filter := bson.D{{"name", "test_auto_idx"}}
		cursor, err := session.Database("testdata").ListCollections(ctx, filter)
		require.NoError(t, err, "should list the collections")

		defer cursor.Close(ctx)

		documentExists := cursor.Next(ctx)
		require.True(t, documentExists, "should find the restored collection")

		var collInfo struct {
			Options bson.M
		}
		err = cursor.Decode(&collInfo)
		require.NoError(t, err, "should decode the collection info")

		if restore.serverVersion.GTE(db.Version{4, 0, 0}) {
			assert.Equal(
				t,
				true,
				collInfo.Options["autoIndexId"],
				"autoIndexId should be flipped to true for server version >= 4.0",
			)
		} else {
			assert.Equal(
				t,
				false,
				collInfo.Options["autoIndexId"],
				"autoIndexId should remain false for server version < 4.0",
			)
		}
	})

	dbName := session.Database("testdata")

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer dbName.Collection("test_auto_idx").Drop(ctx)

	args := []string{
		PreserveUUIDOption, "1",
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	if restore.serverVersion.GTE(db.Version{4, 0, 0}) {
		t.Run("set --preserveUUID if server version >= 4.0", func(t *testing.T) {
			restore.TargetDirectory = "testdata/test_auto_idx.bson"
			result := restore.Restore()
			require.NoError(t, result.Err, "should restore without error")

			// Find the collection
			filter := bson.D{{"name", "test_auto_idx"}}
			cursor, err := session.Database("testdata").ListCollections(ctx, filter)
			require.NoError(t, err, "should list the collections")

			defer cursor.Close(ctx)

			documentExists := cursor.Next(ctx)
			require.True(t, documentExists, "should find the restored collection")

			var collInfo struct {
				Options bson.M
			}
			err = cursor.Decode(&collInfo)
			require.NoError(t, err, "should decode the collection info")

			// restore.serverVersion.GTE(db.Version{4, 0, 0}) is already true here
			// (it gates the enclosing if), so this always takes the true branch;
			// preserved as the original's shape rather than collapsed, since the
			// original re-checked it as a nested Convey in the same way.
			if restore.serverVersion.GTE(db.Version{4, 0, 0}) {
				assert.Equal(
					t,
					true,
					collInfo.Options["autoIndexId"],
					"autoIndexId should be flipped to true for server version >= 4.0",
				)
			} else {
				assert.Equal(
					t,
					false,
					collInfo.Options["autoIndexId"],
					"autoIndexId should remain false for server version < 4.0",
				)
			}
		})
	}
}

// TestSkipSystemCollections asserts that certain system collections like "config.systems.sessions" and the transaction
// related tables aren't applied via applyops when replaying the oplog.
func TestSkipSystemCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "must get a client from the session provider")

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.SkipNow()
	}

	_, err = sessionProvider.GetNodeType()
	require.NoError(t, err, "should get the node type")

	db3 := session.Database("db3")

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer db3.Collection("c1").Drop(ctx)

	args := []string{
		DirectoryOption, "testdata/oplog_partial_skips",
		OplogReplayOption,
		DropOption,
	}

	currentTS := uint32(time.Now().UTC().Unix())

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")

	queryObj := bson.D{
		{"$and",
			bson.A{
				bson.D{{"ts", bson.M{"$gte": bson.Timestamp{T: currentTS, I: 1}}}},
				bson.D{{"$or", bson.A{
					bson.D{
						{"ns", bson.Regex{Pattern: "^config.system.sessions*"}},
					},
					bson.D{{"ns", bson.Regex{Pattern: "^config.cache.*"}}},
				}}},
			},
		},
	}

	cursor, err := session.Database("local").
		Collection("oplog.rs").
		Find(t.Context(), queryObj, nil)
	require.NoError(t, err, "should query the oplog")

	flag := cursor.Next(ctx)
	assert.False(t, flag, "applyOps should skip system-related collections during mongorestore")

	cursor.Close(ctx)
}

// TestSkipStartAndAbortIndexBuild asserts that all "startIndexBuild" and "abortIndexBuild" oplog
// entries are skipped when restoring the oplog.
func TestSkipStartAndAbortIndexBuild(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "must get a client from the session provider")

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.SkipNow()
	}

	testdb := session.Database("test")

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer testdb.Collection("skip_index_entries").Drop(ctx)

	// oplog.bson only has startIndexBuild and abortIndexBuild entries
	args := []string{
		DirectoryOption, "testdata/oplog_ignore_index",
		OplogReplayOption,
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	if !restore.serverVersion.GTE(db.Version{4, 4, 0}) {
		return
	}

	// Run mongorestore
	dbLocal := session.Database("local")
	queryObj := bson.D{{
		"and", bson.A{
			bson.D{{"ns", bson.M{"$ne": "config.system.sessions"}}},
			bson.D{{"op", bson.M{"$ne": "n"}}},
		},
	}}

	countBeforeRestore, err := dbLocal.Collection("oplog.rs").CountDocuments(ctx, queryObj)
	require.NoError(t, err, "should count oplog entries before restore")

	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")

	// Filter out no-ops
	countAfterRestore, err := dbLocal.Collection("oplog.rs").
		CountDocuments(ctx, queryObj)
	require.NoError(t, err, "should count oplog entries after restore")

	assert.Equal(
		t,
		countBeforeRestore,
		countAfterRestore,
		"no new oplog entries should be recorded",
	)
}

// TestcommitIndexBuild asserts that all "commitIndexBuild" are converted to creatIndexes commands.
func TestCommitIndexBuild(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()
	testDB := "commit_index"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "must get a client from the session provider")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "4.4"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV at least 4.4")
	}

	_, err = sessionProvider.GetNodeType()
	require.NoError(t, err, "should get the node type")

	testdb := session.Database(testDB)

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer testdb.Collection(testDB).Drop(ctx)

	args := []string{
		DirectoryOption, "testdata/commit_indexes_build",
		OplogReplayOption,
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")

	destColl := session.Database("commit_index").Collection("test")
	indexes, _ := destColl.Indexes().List(t.Context())

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
	for indexes.Next(t.Context()) {
		var index indexSpec
		err := indexes.Decode(&index)
		require.NoError(t, err, "should decode each index document")
		indexCnt++
	}
	// Should create 3 indexes: _id and two others
	assert.Equal(t, 3, indexCnt, "should create the id index plus two others")
}

// CreateIndexes oplog will be applied directly for versions < 4.4 and converted to createIndex cmd > 4.4.
func TestCreateIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()
	testDB := "create_indexes"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")
	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "must get a client from the session provider")

	_, err = sessionProvider.GetNodeType()
	require.NoError(t, err, "should get the node type")

	testdb := session.Database(testDB)

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer testdb.Collection(testDB).Drop(ctx)

	args := []string{
		DirectoryOption, "testdata/create_indexes",
		OplogReplayOption,
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")

	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.NoError(t, result.Err, "should restore without error")

	destColl := session.Database("create_indexes").Collection("test")
	indexes, _ := destColl.Indexes().List(t.Context())

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
	for indexes.Next(t.Context()) {
		var index indexSpec
		err := indexes.Decode(&index)
		require.NoError(t, err, "should decode each index document")
		indexCnt++
	}
	// Should create 3 indexes: _id and two others
	assert.Equal(t, 3, indexCnt, "should create the id index plus two others")
}

func TestGeoHaystackIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := t.Context()
	dbName := "geohaystack_test"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "must connect to the cluster")

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "must get a client from the session provider")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 5.0 or later")
	}

	testdb := session.Database(dbName)

	// Drop the collection to clean up resources
	//
	//nolint:errcheck
	defer testdb.Collection("foo").Drop(ctx)

	args := []string{
		DirectoryOption, "testdata/coll_with_geohaystack_index",
		DropOption,
	}

	restore, err := getRestoreWithArgs(args...)
	require.NoError(t, err, "should build a restore instance")
	defer restore.Close()

	// Run mongorestore
	result := restore.Restore()
	require.Error(t, result.Err, "should fail to restore a geoHaystack index")

	assert.Contains(
		t,
		result.Err.Error(),
		"found a geoHaystack index",
		"error should mention the geoHaystack index",
	)
}

func createTimeseries(t *testing.T, dbName, coll string, client *mongo.Client) {
	timeseriesOptions := bson.M{
		"timeField": "ts",
		"metaField": "meta",
	}
	createCmd := bson.D{
		{"create", coll},
		{"timeseries", timeseriesOptions},
	}
	client.Database(dbName).RunCommand(t.Context(), createCmd)
}

func TestRestoreTimeseriesCollections(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	dbName := "timeseries_test"

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "no cluster available")

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "no client available")

	fcv := testutil.GetFCV(session)
	if cmp, err := testutil.CompareFCV(fcv, "5.0"); err != nil || cmp < 0 {
		t.Skip("Requires server with FCV 5.0 or later")
	}

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "can get server version")

	testdb := session.Database(dbName)
	dataColl := testdb.Collection("foo_ts")
	bucketsColl := testdb.Collection(common.TimeseriesBucketPrefix + "foo_ts")

	// countBuckets returns the number of underlying timeseries buckets for the given logical
	// collection. On servers that support the rawData API (8.3+), the buckets are no longer
	// accessible via a separate `system.buckets.*` namespace; direct access is rejected with
	// CommandNotSupportedOnLegacyTimeseriesBucketsNamespace. Instead we read them from the logical
	// collection using the rawData option.
	countBuckets := func(t *testing.T, logicalColl string) int64 {
		if serverVersion.SupportsRawData() {
			opts := mopt.Count()
			require.NoError(t, xoptions.SetInternalCountOptions(opts, "rawData", true))
			count, err := testdb.Collection(logicalColl).CountDocuments(t.Context(), bson.M{}, opts)
			require.NoError(t, err)
			return count
		}

		count, err := testdb.Collection(common.TimeseriesBucketPrefix+logicalColl).
			CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		return count
	}

	dropTestDB := func(t *testing.T) {
		err := testdb.Drop(t.Context())
		require.NoError(t, err)
	}

	dropTestDB(t)

	// This checks the result and also counts documents in the data & buckets collections.
	assertSuccess := func(t *testing.T, result Result) {
		assert.EqualValues(t, 10, result.Successes)
		assert.EqualValues(t, 0, result.Failures)

		count, err := dataColl.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.EqualValues(t, 1000, count)

		assert.EqualValues(t, 10, countBuckets(t, "foo_ts"))
	}

	// This checks that there are no docs in either the data or buckets collections.
	assertNoDocs := func(t *testing.T) {
		count, err := dataColl.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.Zero(t, count)

		assert.Zero(t, countBuckets(t, "foo_ts"))
	}

	t.Run("normal restore", func(t *testing.T) {
		runTest := func(t *testing.T, restore *MongoRestore) {
			defer dropTestDB(t)

			result := restore.Restore()
			require.NoError(t, err)
			assertSuccess(t, result)
		}

		t.Run("directory", func(t *testing.T) {
			args := []string{DirectoryOption, "testdata/timeseries_tests/ts_dump"}
			restore, err := getRestoreWithArgs(args...)
			require.NoError(t, err)
			runTest(t, restore)
		})

		t.Run("archive", func(t *testing.T) {
			args := []string{ArchiveOption + "=testdata/timeseries_tests/dump.archive"}
			restore, err := getRestoreWithArgs(args...)
			require.NoError(t, err)
			runTest(t, restore)
		})

		t.Run("archive from stdin", func(t *testing.T) {
			args := []string{ArchiveOption + "=-"}
			restore, err := getRestoreWithArgs(args...)
			require.NoError(t, err)

			archiveFile, err := os.Open("testdata/timeseries_tests/dump.archive")
			require.NoError(t, err)

			restore.InputReader = archiveFile
			runTest(t, restore)
		})
	})

	t.Run("failure cases", func(t *testing.T) {
		t.Run("collection exists", func(t *testing.T) {
			defer dropTestDB(t)

			createTimeseries(t, dbName, "foo_ts", session)

			args := []string{DirectoryOption, "testdata/timeseries_tests/ts_dump"}
			restore, err := getRestoreWithArgs(args...)
			require.NoError(t, err)

			result := restore.Restore()
			defer restore.Close()
			require.Error(t, result.Err)
		})

		t.Run("buckets collection exists", func(t *testing.T) {
			defer dropTestDB(t)

			restore, err := getRestoreWithArgs()
			require.NoError(t, err)
			// In the 8.0 release, this no longer leads to an error, so
			// there's nothing to test here.
			if restore.serverVersion.GTE(db.Version{8, 0, 0}) {
				t.Skip("this tests a pre-8.0 timeseries behavior")
				return
			}

			testdb.RunCommand(t.Context(), bson.M{"create": bucketsColl.Name()})

			result := restore.Restore()
			defer restore.Close()
			require.Error(t, result.Err)
		})
	})

	t.Run("oplogReplay and system.buckets", func(t *testing.T) {
		// This fixture's oplog contains CRUD ops against a legacy `system.buckets.*` namespace
		// (dumped from a pre-viewless server). Servers that support the rawData API (8.3+) use
		// viewless timeseries collections where that namespace no longer exists, and applyOps
		// cannot apply those ops to the logical collection (it rejects the rawData option). So
		// replaying a legacy system.buckets oplog into a viewless timeseries collection is
		// unsupported.
		if serverVersion.SupportsRawData() {
			t.Skip(
				"legacy system.buckets oplog replay is unsupported on viewless timeseries servers (8.3+)",
			)
		}

		defer dropTestDB(t)

		args := []string{
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump_with_oplog",
			OplogReplayOption,
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assert.EqualValues(t, 10, result.Successes)
		assert.Zero(t, 0, result.Failures)

		count, err := dataColl.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.EqualValues(t, 2164, count)

		count, err = bucketsColl.CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.EqualValues(t, 10, count)
	})

	t.Run("existing coll with --drop", func(t *testing.T) {
		defer dropTestDB(t)

		createTimeseries(t, dbName, "foo_ts", session)
		args := []string{
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
			DropOption,
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assertSuccess(t, result)
	})

	t.Run("--noOptionsRestore", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
			NoOptionsRestoreOption,
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.Error(t, result.Err)
	})

	t.Run("invalid system.buckets", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump_invalid_buckets",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assert.Zero(t, result.Successes)
		assert.EqualValues(t, 5, result.Failures)

		assertNoDocs(t)
	})

	t.Run("invalid system.buckets with bypassDocumentValidation", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump_invalid_buckets",
			BypassDocumentValidationOption,
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, err)
		assert.Zero(t, result.Successes)
		assert.EqualValues(t, 5, result.Failures)

		assertNoDocs(t)
	})

	t.Run("system.buckets BSON with metadata", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DBOption,
			dbName,
			CollectionOption,
			"foo_ts",
			"testdata/timeseries_tests/ts_dump/timeseries_test/system.buckets.foo_ts.bson",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assertSuccess(t, result)
	})

	t.Run("system.buckets BSON with metadata and --collection", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DBOption,
			dbName,
			CollectionOption,
			"bar_ts",
			"testdata/timeseries_tests/ts_dump/timeseries_test/system.buckets.foo_ts.bson",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assert.EqualValues(t, 10, result.Successes)
		assert.Zero(t, result.Failures)

		count, err := testdb.Collection("bar_ts").CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.EqualValues(t, 1000, count)

		assert.EqualValues(t, 10, countBuckets(t, "bar_ts"))
	})

	t.Run("system.buckets BSON file as target with metadata", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			"testdata/timeseries_tests/ts_dump/timeseries_test/system.buckets.foo_ts.bson",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assertSuccess(t, result)
	})

	t.Run("system.buckets BSON file without metadata", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DBOption,
			dbName,
			CollectionOption,
			"system.buckets.foo_ts",
			"testdata/timeseries_tests/ts_single_buckets_file/system.buckets.foo_ts.bson",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.Error(t, result.Err)
	})

	t.Run("system.buckets with --nsInclude", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			NSIncludeOption,
			dbName + ".foo_ts",
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assertSuccess(t, result)
	},
	)

	t.Run("system.buckets with not-included collection", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			NSIncludeOption,
			dbName + "." + bucketsColl.Name(),
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assert.Zero(t, result.Successes)
		assert.Zero(t, result.Failures)
		assertNoDocs(t)
	})

	t.Run("system.buckets with excluded collection", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			NSExcludeOption,
			dbName + ".foo_ts",
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assert.Zero(t, result.Successes)
		assert.Zero(t, result.Failures)
		assertNoDocs(t)
	})

	t.Run("system.buckets clustered index with --noIndexRestore", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
			NoIndexRestoreOption,
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assertSuccess(t, result)

		ctx := t.Context()

		indexes, err := testdb.Collection("foo_ts").Indexes().List(ctx)
		require.NoError(t, err)
		defer indexes.Close(ctx)

		numIndexes := 0
		for indexes.Next(ctx) {
			numIndexes++
		}

		if (restore.serverVersion.GTE(db.Version{6, 3, 0})) {
			assert.EqualValues(
				t,
				numIndexes,
				1,
				"--noIndexRestore should build the index on meta, time by default for time-series collections if server version >= 6.3.0",
			)
		} else {
			assert.Zero(t, numIndexes)
		}

		cur, err := testdb.ListCollections(ctx, bson.M{"name": bucketsColl.Name()})
		require.NoError(t, err)

		for cur.Next(ctx) {
			optVal, err := cur.Current.LookupErr("options")
			require.NoError(t, err)

			optRaw, ok := optVal.DocumentOK()
			require.True(t, ok)

			clusteredIdxVal, err := optRaw.LookupErr("clusteredIndex")
			require.NoError(t, err)

			clusteredIdx := clusteredIdxVal.Boolean()
			assert.True(t, clusteredIdx)
		}
	})

	t.Run("rename system.buckets", func(t *testing.T) {
		defer dropTestDB(t)

		args := []string{
			NSFromOption,
			dbName + ".foo_ts",
			NSToOption,
			dbName + ".foo_rename_ts",
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assert.EqualValues(t, 10, result.Successes)
		assert.Zero(t, result.Failures)

		assertNoDocs(t)

		count, err := testdb.Collection("foo_rename_ts").
			CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.EqualValues(t, 1000, count)

		assert.EqualValues(t, 10, countBuckets(t, "foo_rename_ts"))
	})

	t.Run("refuse rename of system.bucket", func(t *testing.T) {
		// The system.buckets collection should not be renamed if the timeseries collection is not
		// renamed, even if the user tries to rename the system.buckets collection.
		defer dropTestDB(t)

		renameBucketsName := common.TimeseriesBucketPrefix + "foo_rename_ts"

		args := []string{
			NSFromOption,
			dbName + "." + bucketsColl.Name(),
			NSToOption,
			dbName + "." + renameBucketsName,
			DirectoryOption,
			"testdata/timeseries_tests/ts_dump",
		}
		restore, err := getRestoreWithArgs(args...)
		require.NoError(t, err)

		result := restore.Restore()
		defer restore.Close()
		require.NoError(t, result.Err)
		assertSuccess(t, result)

		count, err := testdb.Collection("foo_rename_ts").
			CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		assert.Zero(t, count)

		assert.Zero(t, countBuckets(t, "foo_rename_ts"))
	})
}

type indexInfo struct {
	name string
	keys []string
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

	res := session.Database("admin").RunCommand(t.Context(), bson.M{"replSetGetStatus": 1})
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
		err = testDB.Drop(t.Context())
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
	}()

	dataLen := createClusteredIndex(t, testDB, indexName)

	withBSONMongodumpForCollection(t, testDB.Name(), "stocks", func(dir string) {
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
		err = testDB.Drop(t.Context())
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
	res := testDB.RunCommand(t.Context(), createCollCmd, nil)
	require.NoError(res.Err(), "can create a clustered collection")

	var r any
	err := res.Decode(&r)
	require.NoError(err)

	stocks := testDB.Collection("stocks")
	stockData := []any{
		bson.M{"ticker": "MDB", "price": 245.33},
		bson.M{"ticker": "GOOG", "price": 2214.91},
		bson.M{"ticker": "BLZE", "price": 6.23},
	}
	_, err = stocks.InsertMany(t.Context(), stockData)
	require.NoError(err, "can insert documents into collection")

	return len(stockData)
}

func assertClusteredIndex(t *testing.T, testDB *mongo.Database, indexName string) {
	require := require.New(t)

	c, err := testDB.ListCollections(t.Context(), bson.M{})
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
	for c.Next(t.Context()) {
		var res collectionRes
		decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(c.Current)))
		decoder.DefaultDocumentM()
		err = decoder.Decode(&res)
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

	//nolint:errcheck
	idxM := idx.(bson.M)
	name, found := idxM["name"]
	require.True(t, found, "index has a key named 'name'")
	require.IsType(t, "string", name, "key value is a string")

	keys, found := idxM["key"]
	require.True(t, found, "index has a key named 'key'")
	require.IsType(t, bson.M{}, keys, "key value is a bson.M")

	keysM, ok := keys.(bson.M)
	require.True(t, ok)

	var keyNames []string
	for k := range keysM {
		keyNames = append(keyNames, k)
	}

	nameStr, ok := name.(string)
	require.True(t, ok)

	return indexInfo{
		name: nameStr,
		keys: keyNames,
	}
}

func withBSONMongodumpForCollection(
	t *testing.T,
	db string,
	collection string,
	testCase func(string),
) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	runBSONMongodumpForCollection(t, dir, db, collection)
	testCase(dir)
}

func withOplogMongoDump(t *testing.T, db string, collection string, testCase func(string)) {
	require := require.New(t)

	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	// This queries the local.oplog.rs collection for commands or CRUD
	// operations on the collection we are testing (which will have a unique
	// name for each test).
	query := map[string]any{
		"$or": []map[string]string{
			{"ns": fmt.Sprintf("%s.$cmd", db)},
			{"ns": fmt.Sprintf("%s.%s", db, collection)},
		},
	}
	q, err := json.Marshal(query)
	require.NoError(err, "can marshal query to JSON")

	// We dump just the documents matching the query using mongodump "normally".
	bsonFile := runBSONMongodumpForCollection(t, dir, "local", "oplog.rs", "--query", string(q))

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

func runBSONMongodumpForCollection(
	t *testing.T,
	dir, db, collection string,
	args ...string,
) string {
	require := require.New(t)
	baseArgs := []string{
		"--out", dir,
		"--db", db,
		"--collection", collection,
	}
	runMongodumpWithArgs(
		t,
		append(baseArgs, args...)...,
	)
	bsonFile := filepath.Join(dir, db, fmt.Sprintf("%s.bson", collection))
	_, err := os.Stat(bsonFile)
	require.NoError(err, "dump created BSON data file")
	_, err = os.Stat(filepath.Join(dir, db, fmt.Sprintf("%s.metadata.json", collection)))
	require.NoError(err, "dump created JSON metadata file")
	return bsonFile
}

func runMongodumpWithArgs(t *testing.T, args ...string) {
	require := require.New(t)
	cmd := []string{"go", "run", filepath.Join("..", "mongodump", "main")}
	cmd = append(cmd, testutil.GetBareArgs()...)
	cmd = append(cmd, args...)
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	cmdStr := strings.Join(cmd, " ")
	require.NoError(err, "can execute command %s with output: %s", cmdStr, out)
	require.NotContains(
		string(out),
		"does not exist",
		"running [%s] does not tell us the namespace does not exist",
		cmdStr,
	)

	// So we can see dump’s output when debugging test failures:
	fmt.Print(string(out))
}

func uniqueDBName() string {
	return fmt.Sprintf("mongorestore_test_%d_%d", os.Getpid(), time.Now().UnixMilli())
}
