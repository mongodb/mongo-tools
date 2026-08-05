package dumprestore

import (
	"os"
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	cappedSize = 1000
	cappedMax  = 10

	// Far more documents than the capped collection can hold, so the server
	// evicts as it inserts and the resulting count is the cap itself.
	cappedInsertCount = 1000
)

// TestCappedCollectionRoundTrip checks that a capped collection's options,
// indexes and eviction behavior survive a dump and restore, at each of the
// granularities a restore can be asked for.
func (s *DumpRestoreSuite) TestCappedCollectionRoundTrip() {
	s.Run("full dump directory", s.testCappedRoundTripFromDumpRoot)
	s.Run("single database directory into a new database", s.testCappedRoundTripIntoNewDB)
	s.Run("nsInclude for one database", s.testCappedRoundTripWithNSInclude)
	s.Run("single collection bson file into a new collection", s.testCappedRoundTripSingleColl)
}

func (s *DumpRestoreSuite) testCappedRoundTripFromDumpRoot() {
	testDB := s.database("capped_root")
	fixture := s.createCappedFixture(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore from the dump root")
	}, "--db", testDB.Name())

	s.assertCappedFixtureRestored(testDB, fixture)
}

func (s *DumpRestoreSuite) testCappedRoundTripIntoNewDB() {
	testDB := s.database("capped_new_db")
	fixture := s.createCappedFixture(testDB)
	restoredDB := s.database("capped_new_db_restored")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.DBOption, restoredDB.Name(),
			mongorestore.DirectoryOption, filepath.Join(dir, testDB.Name()),
		)
		s.Require().NoError(result.Err, "can restore a database directory under a new name")
	}, "--db", testDB.Name())

	s.assertCappedFixtureRestored(restoredDB, fixture)
}

func (s *DumpRestoreSuite) testCappedRoundTripWithNSInclude() {
	testDB := s.database("capped_nsinclude")
	fixture := s.createCappedFixture(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.NSIncludeOption, testDB.Name()+".*",
			dir,
		)
		s.Require().NoError(result.Err, "can restore with --nsInclude")
	}, "--db", testDB.Name())

	s.assertCappedFixtureRestored(testDB, fixture)
}

func (s *DumpRestoreSuite) testCappedRoundTripSingleColl() {
	testDB := s.database("capped_single_coll")
	fixture := s.createCappedFixture(testDB)
	restoredColl := testDB.Collection("baz")

	s.withBSONMongodumpForCollection(testDB.Name(), "capped", func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.DBOption, testDB.Name(),
			mongorestore.CollectionOption, restoredColl.Name(),
			filepath.Join(dir, testDB.Name(), "capped.bson"),
		)
		s.Require().NoError(result.Err, "can restore a single collection under a new name")
	})

	s.assertCappedCollectionRestored(restoredColl, fixture)
}

// cappedFixture records what the capped collection looked like before the dump,
// so the restored collection can be compared against it. The server rounds the
// requested capped size, so the options must come from the server rather than
// from the values the fixture asked for.
type cappedFixture struct {
	docCount int64
	options  bson.D
}

// createCappedFixture creates a plain collection with two secondary indexes and
// a capped collection that has already evicted documents.
func (s *DumpRestoreSuite) createCappedFixture(testDB *mongo.Database) cappedFixture {
	ctx := s.Context()

	plainColl := testDB.Collection("plain")
	_, err := plainColl.InsertOne(ctx, bson.D{{"a", 1}, {"b", 1}})
	s.Require().NoError(err, "can insert into the plain collection")

	_, err = plainColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{"a", 1}}},
		{Keys: bson.D{{"b", 1}, {"_id", -1}}},
	})
	s.Require().NoError(err, "can create indexes on the plain collection")

	cappedColl := s.createCollection(
		testDB,
		"capped",
		options.CreateCollection().
			SetCapped(true).
			SetSizeInBytes(cappedSize).
			SetMaxDocuments(cappedMax),
	)

	docs := make([]any, cappedInsertCount)
	for i := range docs {
		docs[i] = bson.D{{"x", i}}
	}
	_, err = cappedColl.InsertMany(ctx, docs)
	s.Require().NoError(err, "can insert into the capped collection")

	_, err = cappedColl.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{"x", 1}}})
	s.Require().NoError(err, "can create an index on the capped collection")

	cappedOptions := s.collectionOptions(testDB, cappedColl.Name())
	s.Require().
		Equal(true, optionValue(cappedOptions, "capped"), "the fixture collection really is capped")

	cappedCount := s.docCount(cappedColl)
	s.Require().Positive(cappedCount, "the capped collection retained documents")
	s.Require().Less(
		cappedCount,
		int64(cappedInsertCount),
		"the capped collection evicted documents",
	)

	s.Assert().Len(
		append(s.indexNames(plainColl), s.indexNames(cappedColl)...),
		5,
		"the fixture created every index",
	)

	return cappedFixture{docCount: cappedCount, options: cappedOptions}
}

func (s *DumpRestoreSuite) createCollection(
	testDB *mongo.Database,
	collName string,
	opts *options.CreateCollectionOptionsBuilder,
) *mongo.Collection {
	err := testDB.CreateCollection(s.Context(), collName, opts)
	s.Require().NoError(err, "can create the collection %#q", collName)

	return testDB.Collection(collName)
}

func (s *DumpRestoreSuite) assertCappedFixtureRestored(
	testDB *mongo.Database,
	fixture cappedFixture,
) {
	plainColl := testDB.Collection("plain")
	cappedColl := testDB.Collection("capped")

	s.Assert().EqualValues(1, s.docCount(plainColl), "the plain collection is restored")
	s.Assert().ElementsMatch(
		[]string{"_id_", "a_1", "b_1__id_-1"},
		s.indexNames(plainColl),
		"the plain collection's indexes are restored",
	)

	s.assertCappedCollectionRestored(cappedColl, fixture)

	// Only the capped fields are compared. Newer servers report additional
	// default options that mongodump does not carry in its metadata, so
	// comparing the whole options document breaks on server upgrades.
	restoredOptions := s.collectionOptions(testDB, cappedColl.Name())
	for _, field := range []string{"capped", "size", "max"} {
		s.Assert().EqualValues(
			optionValue(fixture.options, field),
			optionValue(restoredOptions, field),
			"the %#q option survives the round trip",
			field,
		)
	}
}

// assertCappedCollectionRestored checks the capped collection of the fixture,
// which is restored under its own name in most of these tests and under a new
// one when a single collection is restored.
func (s *DumpRestoreSuite) assertCappedCollectionRestored(
	coll *mongo.Collection,
	fixture cappedFixture,
) {
	s.Assert().EqualValues(
		fixture.docCount,
		s.docCount(coll),
		"the capped collection is restored into %#q with all of its documents",
		coll.Name(),
	)
	s.Assert().ElementsMatch(
		[]string{"_id_", "x_1"},
		s.indexNames(coll),
		"the capped collection's indexes are restored into %#q",
		coll.Name(),
	)
	s.assertCappedEvictionWorks(coll, fixture.docCount)
}

// assertCappedEvictionWorks inserts more documents than the cap allows and
// verifies the count does not grow, which only holds if the restored collection
// is genuinely capped.
func (s *DumpRestoreSuite) assertCappedEvictionWorks(coll *mongo.Collection, wantCount int64) {
	docs := make([]any, cappedMax)
	for i := range docs {
		docs[i] = bson.D{{"x", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert into the restored capped collection")

	s.Assert().EqualValues(
		wantCount,
		s.docCount(coll),
		"the restored capped collection still evicts documents",
	)
}

const extJSONMetadataCappedSize = 10_000_000

// TestRestoreExtendedJSONMetadata restores metadata of the shape that very old
// versions of mongodump wrote: legacy extended JSON types for numeric values in
// both the collection options and the index specs, v1 indexes, and an "ns"
// field. Modern mongodump cannot produce such a dump, so the metadata is
// marshaled as canonical extended JSON here, which wraps every numeric value the
// way those old dumps did.
func (s *DumpRestoreSuite) TestRestoreExtendedJSONMetadata() {
	const collName = "changelog"

	testDB := s.database("extjson_metadata")
	dir, dbDir := s.newDumpDir(testDB.Name())
	ns := testDB.Name() + "." + collName

	metadata, err := bson.MarshalExtJSON(bson.D{
		{"options", bson.D{{"size", int64(extJSONMetadataCappedSize)}, {"capped", true}}},
		{"indexes", bson.A{
			bson.D{
				{"v", 1},
				{"key", bson.D{{"_id", int64(1)}}},
				{"ns", ns},
				{"name", "_id_"},
			},
			bson.D{
				{"v", 1},
				{"key", bson.D{{"pos", "2d"}}},
				{"ns", ns},
				{"name", "position_2d"},
				{"min", int64(0)},
				{"max", int64(1000)},
				{"bits", int64(32)},
			},
		}},
	}, true, false)
	s.Require().NoError(err, "can marshal the metadata fixture")

	s.Require().NoError(
		os.WriteFile(filepath.Join(dbDir, collName+".metadata.json"), metadata, 0644),
		"can write the metadata fixture",
	)
	s.writeBSONFile(
		filepath.Join(dbDir, collName+".bson"),
		bson.D{{"_id", 1}, {"what", "dropCollection"}},
		bson.D{{"_id", 2}, {"what", "shardCollection"}},
	)

	result := s.runRestore(dir)
	s.Require().NoError(result.Err, "can restore a dump with extended JSON metadata")

	coll := testDB.Collection(collName)
	s.Assert().EqualValues(2, s.docCount(coll), "the documents are restored")

	restoredOptions := s.collectionOptions(testDB, collName)
	s.Assert().Equal(true, optionValue(restoredOptions, "capped"), "the capped option is restored")
	// The server rounds the capped size to a storage-engine-friendly value.
	s.Assert().InDelta(
		extJSONMetadataCappedSize,
		optionValue(restoredOptions, "size"),
		1000,
		"the capped size is restored from an extended JSON $numberLong",
	)

	s.Assert().ElementsMatch(
		[]string{"_id_", "position_2d"},
		s.indexNames(coll),
		"indexes with extended JSON specs are restored",
	)

	idKey := s.indexKey(coll, "_id_")
	s.Require().Len(idKey, 1, "the _id index has a single key field")
	s.Assert().Equal("_id", idKey[0].Key, "the _id index is keyed on _id")
	s.Assert().EqualValues(
		1,
		idKey[0].Value,
		"the extended JSON $numberLong index key is read as a plain number",
	)

	geoKey := s.indexKey(coll, "position_2d")
	s.Require().Len(geoKey, 1, "the 2d index has a single key field")
	s.Assert().Equal("pos", geoKey[0].Key, "the 2d index is keyed on pos")
	s.Assert().Equal("2d", geoKey[0].Value, "the 2d index keeps its index type")
}

const (
	optionsFixtureDocCount = 50

	// Large enough to hold every fixture document, so a document count check
	// cannot pass just because the collection is capped.
	optionsFixtureCappedSize = 4096
)

var optionsFixtureCollNames = []string{"capped", "validated", "plain"}

// TestNoOptionsRestore checks that a default restore recreates a collection
// with the options it was dumped with, while --noOptionsRestore recreates it
// with none of them.
func (s *DumpRestoreSuite) TestNoOptionsRestore() {
	s.Run("default restore preserves collection options", s.testRestorePreservesOptions)
	s.Run("noOptionsRestore strips collection options", s.testNoOptionsRestoreStripsOptions)
	s.Run("noOptionsRestore strips options for one collection", s.testNoOptionsRestoreOneColl)
}

func (s *DumpRestoreSuite) testRestorePreservesOptions() {
	testDB := s.database("options_preserved")
	s.createOptionsFixture(testDB)

	cappedOptions := s.collectionOptions(testDB, "capped")
	validatedOptions := s.collectionOptions(testDB, "validated")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore the dump")
	}, "--db", testDB.Name())

	s.assertOptionsFixtureDataRestored(testDB)

	restoredCapped := s.collectionOptions(testDB, "capped")
	for _, field := range []string{"capped", "size"} {
		s.Assert().EqualValues(
			optionValue(cappedOptions, field),
			optionValue(restoredCapped, field),
			"the %#q option survives the round trip",
			field,
		)
	}

	s.Assert().EqualValues(
		optionValue(validatedOptions, "validator"),
		s.collectionOption(testDB, "validated", "validator"),
		"the validator survives the round trip",
	)
}

func (s *DumpRestoreSuite) testNoOptionsRestoreStripsOptions() {
	testDB := s.database("options_stripped")
	s.createOptionsFixture(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(mongorestore.NoOptionsRestoreOption, dir)
		s.Require().NoError(result.Err, "can restore with --noOptionsRestore")
	}, "--db", testDB.Name())

	s.assertOptionsFixtureDataRestored(testDB)

	for _, collName := range optionsFixtureCollNames {
		s.assertOptionsStripped(testDB, collName)
	}
}

// assertOptionsStripped checks that none of the options the fixture set survive.
// It names the options rather than comparing the whole document against a plain
// collection's, because servers grow new default option fields over time and
// report them for every collection.
func (s *DumpRestoreSuite) assertOptionsStripped(testDB *mongo.Database, collName string) {
	for _, field := range []string{"capped", "size", "max", "validator"} {
		s.Assert().Nil(
			s.collectionOption(testDB, collName, field),
			"--noOptionsRestore leaves %#q with no %#q option",
			collName,
			field,
		)
	}
}

func (s *DumpRestoreSuite) testNoOptionsRestoreOneColl() {
	testDB := s.database("options_stripped_coll")
	s.createOptionsFixture(testDB)

	s.withBSONMongodumpForCollection(testDB.Name(), "capped", func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.NoOptionsRestoreOption,
			mongorestore.DBOption, testDB.Name(),
			mongorestore.CollectionOption, "capped",
			filepath.Join(dir, testDB.Name(), "capped.bson"),
		)
		s.Require().NoError(result.Err, "can restore one collection with --noOptionsRestore")
	})

	s.Assert().EqualValues(
		optionsFixtureDocCount,
		s.docCount(testDB.Collection("capped")),
		"the documents are restored",
	)
	s.assertOptionsStripped(testDB, "capped")
}

// createOptionsFixture creates a capped collection, a collection with a
// validator, and a collection with no options at all, each holding the same
// documents.
func (s *DumpRestoreSuite) createOptionsFixture(testDB *mongo.Database) {
	s.createCollection(
		testDB,
		"capped",
		options.CreateCollection().SetCapped(true).SetSizeInBytes(optionsFixtureCappedSize),
	)
	s.createCollection(
		testDB,
		"validated",
		options.CreateCollection().SetValidator(bson.D{{"phone", bson.D{{"$type", "string"}}}}),
	)

	cappedOptions := s.collectionOptions(testDB, "capped")
	s.Require().Equal(
		true,
		optionValue(cappedOptions, "capped"),
		"the fixture collection really is capped",
	)
	s.Require().EqualValues(
		optionsFixtureCappedSize,
		optionValue(cappedOptions, "size"),
		"the fixture collection has the requested capped size",
	)
	s.Require().NotNil(
		s.collectionOption(testDB, "validated", "validator"),
		"the fixture collection really has a validator",
	)

	docs := make([]any, optionsFixtureDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", i}, {"phone", "abc"}}
	}

	for _, collName := range optionsFixtureCollNames {
		_, err := testDB.Collection(collName).InsertMany(s.Context(), docs)
		s.Require().NoError(err, "can insert into %#q", collName)
	}
}

func (s *DumpRestoreSuite) assertOptionsFixtureDataRestored(testDB *mongo.Database) {
	for _, collName := range optionsFixtureCollNames {
		s.Assert().EqualValues(
			optionsFixtureDocCount,
			s.docCount(testDB.Collection(collName)),
			"all documents are restored into %#q",
			collName,
		)
	}
}

// TestCollationRoundTrip checks that a collection's default collation survives a
// dump and restore, both when restoring from the dump directory and when
// restoring the collection's bson file directly.
func (s *DumpRestoreSuite) TestCollationRoundTrip() {
	const collName = "coll"

	testDB := s.database("collation")
	coll := s.createCollection(
		testDB,
		collName,
		options.CreateCollection().SetCollation(&options.Collation{Locale: "fr_CA"}),
	)

	collation := s.collectionOption(testDB, collName, "collation")
	s.Require().NotNil(collation, "the created collection reports a collation")

	s.withBSONMongodump(func(dir string) {
		s.Run("restore from the dump directory", func() {
			s.dropCollection(coll)

			result := s.runRestore(dir)
			s.Require().NoError(result.Err, "can restore from the dump directory")

			s.Assert().Equal(
				collation,
				s.collectionOption(testDB, collName, "collation"),
				"the default collation survives the round trip",
			)
		})

		s.Run("restore from the bson file", func() {
			s.dropCollection(coll)

			result := s.runRestore(filepath.Join(dir, testDB.Name(), collName+".bson"))
			s.Require().NoError(result.Err, "can restore from the bson file")

			s.Assert().Equal(
				collation,
				s.collectionOption(testDB, collName, "collation"),
				"the default collation survives a single-file restore",
			)
		})
	}, "--db", testDB.Name())
}

const (
	validationCollName            = "bar"
	validationFixtureDocCount     = 1000
	validationFixtureValidCount   = validationFixtureDocCount / 2
	validationFixtureInvalidCount = validationFixtureDocCount - validationFixtureValidCount
)

var validationFixtureValidator = bson.D{{"baz", bson.D{{"$exists", true}}}}

// TestRestoreDocumentValidation checks how a restore behaves against a
// collection with a document validator: which documents the server rejects,
// which options let them through, and which options turn a rejection into a
// failed restore.
func (s *DumpRestoreSuite) TestRestoreDocumentValidation() {
	s.Run("a validator on the target drops invalid documents", s.testValidatorDropsInvalidDocs)
	s.Run("bypassDocumentValidation restores invalid documents", s.testBypassDocumentValidation)
	s.Run("a restored validator drops invalid documents", s.testRestoredValidatorDropsInvalidDocs)
	s.Run("bypassDocumentValidation overrides a restored validator", s.testBypassRestoredValidator)
	s.Run("stopOnError fails on a validation error", s.testStopOnErrorWithValidator)
	s.Run("maintainInsertionOrder fails on a validation error", s.testMaintainOrderWithValidator)
}

func (s *DumpRestoreSuite) testValidatorDropsInvalidDocs() {
	testDB := s.database("validation_target")
	s.createValidationFixture(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)
		s.createValidatedCollection(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "restoring against a validator still succeeds")
		s.Assert().EqualValues(
			validationFixtureInvalidCount,
			result.Failures,
			"mongorestore reports the rejected documents as failures",
		)
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationFixtureValidCount,
		s.docCount(testDB.Collection(validationCollName)),
		"only the valid documents are restored",
	)
}

func (s *DumpRestoreSuite) testBypassDocumentValidation() {
	testDB := s.database("validation_bypass")
	s.createValidationFixture(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)
		s.createValidatedCollection(testDB)

		result := s.runRestore(mongorestore.BypassDocumentValidationOption, dir)
		s.Require().NoError(result.Err, "can restore with --bypassDocumentValidation")
		s.Assert().EqualValues(0, result.Failures, "no documents are rejected")
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationFixtureDocCount,
		s.docCount(testDB.Collection(validationCollName)),
		"every document is restored when validation is bypassed",
	)
}

// testRestoredValidatorDropsInvalidDocs checks that the validator itself
// round-trips through the dump's metadata: restoring into an empty database
// recreates it, and the invalid documents in the same dump are then rejected.
func (s *DumpRestoreSuite) testRestoredValidatorDropsInvalidDocs() {
	testDB := s.database("validation_restored")
	s.createValidationFixture(testDB)
	s.addValidatorToCollection(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump that carries a validator")
		s.Assert().EqualValues(
			validationFixtureInvalidCount,
			result.Failures,
			"mongorestore reports the rejected documents as failures",
		)
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationFixtureValidCount,
		s.docCount(testDB.Collection(validationCollName)),
		"only the valid documents are restored",
	)
	s.assertFixtureValidatorRestored(testDB)
}

// testBypassRestoredValidator checks the combination of the two previous cases:
// mongorestore creates the collection with the validator from the dump's
// metadata and must still bypass that validator for its own inserts.
func (s *DumpRestoreSuite) testBypassRestoredValidator() {
	testDB := s.database("validation_bypass_restored")
	s.createValidationFixture(testDB)
	s.addValidatorToCollection(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(mongorestore.BypassDocumentValidationOption, dir)
		s.Require().NoError(result.Err, "can restore a dump that carries a validator")
		s.Assert().EqualValues(0, result.Failures, "no documents are rejected")
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationFixtureDocCount,
		s.docCount(testDB.Collection(validationCollName)),
		"every document is restored when validation is bypassed",
	)
	s.assertFixtureValidatorRestored(testDB)
}

func (s *DumpRestoreSuite) assertFixtureValidatorRestored(testDB *mongo.Database) {
	s.Assert().Equal(
		validationFixtureValidator,
		s.collectionOption(testDB, validationCollName, "validator"),
		"the validator from the dump's metadata is the one on the restored collection",
	)
}

func (s *DumpRestoreSuite) testStopOnErrorWithValidator() {
	s.assertRestoreFailsOnValidationError(
		"validation_stop_on_error",
		mongorestore.StopOnErrorOption,
	)
}

func (s *DumpRestoreSuite) testMaintainOrderWithValidator() {
	s.assertRestoreFailsOnValidationError(
		"validation_maintain_order",
		mongorestore.MaintainInsertionOrderOption,
	)
}

func (s *DumpRestoreSuite) assertRestoreFailsOnValidationError(
	dbName string,
	restoreOption string,
) {
	testDB := s.database(dbName)
	s.createValidationFixture(testDB)

	s.withBSONMongodumpForCollection(testDB.Name(), validationCollName, func(dir string) {
		s.dropDB(testDB)
		s.createValidatedCollection(testDB)

		result := s.runRestore(
			restoreOption,
			mongorestore.DBOption, testDB.Name(),
			mongorestore.CollectionOption, validationCollName,
			filepath.Join(dir, testDB.Name(), validationCollName+".bson"),
		)
		s.Require().ErrorContains(
			result.Err,
			"Document failed validation",
			"%s reports the validation error instead of ignoring it",
			restoreOption,
		)
	})
}

// createValidationFixture inserts documents of which only half satisfy the
// validator that the tests later apply. No validator is in place yet, so all of
// them are inserted.
func (s *DumpRestoreSuite) createValidationFixture(testDB *mongo.Database) {
	docs := make([]any, validationFixtureDocCount)
	for i := range docs {
		doc := bson.D{{"_id", i}, {"num", i + 1}}
		if i%2 == 0 {
			doc = append(doc, bson.E{"baz", i})
		}
		docs[i] = doc
	}

	_, err := testDB.Collection(validationCollName).InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the fixture documents")
}

func (s *DumpRestoreSuite) createValidatedCollection(testDB *mongo.Database) {
	s.createCollection(
		testDB,
		validationCollName,
		options.CreateCollection().SetValidator(validationFixtureValidator),
	)
}

func (s *DumpRestoreSuite) addValidatorToCollection(testDB *mongo.Database) {
	res := testDB.RunCommand(s.Context(), bson.D{
		{"collMod", validationCollName},
		{"validator", validationFixtureValidator},
	})
	s.Require().NoError(res.Err(), "can add a validator to an existing collection")
}

// TestSystemProfileIsNotRestored checks that mongorestore skips a
// system.profile collection in a dump. mongodump never writes one (it skips
// every <db>.system.* namespace), so the dump has to be built by hand to reach
// the restore-side check at all.
func (s *DumpRestoreSuite) TestSystemProfileIsNotRestored() {
	const collName = "coll"

	testDB := s.database("system_profile")
	dir, dbDir := s.newDumpDir(testDB.Name())

	s.writeBSONFile(filepath.Join(dbDir, collName+".bson"), bson.D{{"_id", 1}, {"x", 1}})
	s.writeBSONFile(
		filepath.Join(dbDir, "system.profile.bson"),
		bson.D{{"_id", 1}, {"op", "query"}, {"ns", testDB.Name() + "." + collName}},
	)
	s.Require().NoError(
		os.WriteFile(
			filepath.Join(dbDir, "system.profile.metadata.json"),
			[]byte(`{"options":{},"indexes":[]}`),
			0644,
		),
		"can write the system.profile metadata fixture",
	)

	result := s.runRestore(dir)
	s.Require().NoError(result.Err, "can restore a dump that contains system.profile")

	// Zero failures is what separates "mongorestore skipped the file" from
	// "mongorestore tried to insert and the server rejected it".
	s.Assert().EqualValues(0, result.Failures, "mongorestore reports no failed inserts")
	s.Assert().EqualValues(
		1,
		result.Successes,
		"only the one user document is inserted",
	)

	s.Assert().EqualValues(
		1,
		s.docCount(testDB.Collection(collName)),
		"the user data is restored",
	)
	s.Assert().Zero(
		s.docCount(testDB.Collection("system.profile")),
		"system.profile is not restored",
	)
	s.Assert().NotContains(
		s.collectionNames(testDB),
		"system.profile",
		"the system.profile collection is not even created",
	)
}

// database returns a handle to a database named for the test that asked for it.
// Each test names its own database rather than deriving one from the test name,
// because the suite's DBName truncates to 63 bytes, which leaves too few
// distinguishing characters for these deeply nested subtests.
func (s *DumpRestoreSuite) database(name string) *mongo.Database {
	session, err := testutil.GetBareSession()
	s.Require().NoError(err, "can connect to the server")

	return session.Database("dumprestore_" + name)
}

// newDumpDir creates a dump directory holding one database directory, for tests
// that need a dump mongodump would not produce. It returns the dump root and the
// database directory inside it.
func (s *DumpRestoreSuite) newDumpDir(dbName string) (string, string) {
	root := s.T().TempDir()
	dbDir := filepath.Join(root, dbName)
	s.Require().NoError(os.MkdirAll(dbDir, 0755), "can create the dump directory")

	return root, dbDir
}

// writeBSONFile writes documents in the concatenated-BSON format that mongodump
// produces for a collection.
func (s *DumpRestoreSuite) writeBSONFile(path string, docs ...bson.D) {
	var buf []byte
	for _, doc := range docs {
		marshaled, err := bson.Marshal(doc)
		s.Require().NoError(err, "can marshal a fixture document")
		buf = append(buf, marshaled...)
	}

	s.Require().NoError(os.WriteFile(path, buf, 0644), "can write the bson file %#q", path)
}

func (s *DumpRestoreSuite) runRestore(args ...string) mongorestore.Result {
	restore, err := getRestoreWithArgs(args...)
	s.Require().NoError(err, "can build mongorestore")
	defer restore.Close()

	return restore.Restore()
}

// dropDB drops the database and verifies that it is empty, so that a restore
// which silently does nothing cannot pass by leaving the original data in place.
func (s *DumpRestoreSuite) dropDB(testDB *mongo.Database) {
	s.Require().NoError(
		testDB.Drop(s.Context()),
		"can drop the database %#q",
		testDB.Name(),
	)
	s.Require().Empty(
		s.collectionNames(testDB),
		"dropping %#q removes all of its collections",
		testDB.Name(),
	)
}

func (s *DumpRestoreSuite) dropCollection(coll *mongo.Collection) {
	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection %#q", coll.Name())
	s.Require().NotContains(
		s.collectionNames(coll.Database()),
		coll.Name(),
		"dropping %#q removes it",
		coll.Name(),
	)
}

func (s *DumpRestoreSuite) collectionNames(testDB *mongo.Database) []string {
	names, err := testDB.ListCollectionNames(s.Context(), bson.D{})
	s.Require().NoError(err, "can list the collections in %#q", testDB.Name())

	return names
}

func (s *DumpRestoreSuite) docCount(coll *mongo.Collection) int64 {
	count, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err, "can count documents in %#q", coll.Name())

	return count
}

func (s *DumpRestoreSuite) indexNames(coll *mongo.Collection) []string {
	specs, err := coll.Indexes().ListSpecifications(s.Context())
	s.Require().NoError(err, "can list indexes on %#q", coll.Name())

	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}

	return names
}

// indexKey returns one index's key document as a bson.D, so that the value
// types and field order are visible to assertions.
func (s *DumpRestoreSuite) indexKey(coll *mongo.Collection, indexName string) bson.D {
	cursor, err := coll.Indexes().List(s.Context())
	s.Require().NoError(err, "can list indexes on %#q", coll.Name())

	var indexes []struct {
		Name string `bson:"name"`
		Key  bson.D `bson:"key"`
	}
	s.Require().NoError(cursor.All(s.Context(), &indexes), "can read the index specs")

	for _, index := range indexes {
		if index.Name == indexName {
			return index.Key
		}
	}

	s.Require().Failf(
		"index not found",
		"the index %#q exists on %#q",
		indexName,
		coll.Name(),
	)

	return nil
}

// collectionOptions returns the collection's creation options, which is where
// the server reports capped settings, validators, and collations.
func (s *DumpRestoreSuite) collectionOptions(testDB *mongo.Database, collName string) bson.D {
	cursor, err := testDB.ListCollections(s.Context(), bson.D{{"name", collName}})
	s.Require().NoError(err, "can list collections")

	var infos []struct {
		Options bson.D `bson:"options"`
	}
	s.Require().NoError(cursor.All(s.Context(), &infos), "can read the collection info")
	s.Require().Len(infos, 1, "the collection %#q exists", collName)

	return infos[0].Options
}

// collectionOption returns one field of a collection's options.
func (s *DumpRestoreSuite) collectionOption(
	testDB *mongo.Database,
	collName string,
	key string,
) any {
	return optionValue(s.collectionOptions(testDB, collName), key)
}

// optionValue returns one field of an options document, or nil when the server
// does not report that field at all.
func optionValue(options bson.D, key string) any {
	value, err := bsonutil.FindValueByKey(key, &options)
	if err != nil {
		return nil
	}

	return value
}
