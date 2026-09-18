package dumprestore

import (
	"os"
	"path/filepath"

	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const cappedMax = 10

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
	before := s.createCappedAndPlainCollections(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore from the dump root")
	}, "--db", testDB.Name())

	s.assertCappedAndPlainCollectionsRestored(testDB, before)
}

func (s *DumpRestoreSuite) testCappedRoundTripIntoNewDB() {
	testDB := s.database("capped_new_db")
	before := s.createCappedAndPlainCollections(testDB)
	restoredDB := s.database("capped_new_db_restored")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.DBOption, restoredDB.Name(),
			mongorestore.DirectoryOption, filepath.Join(dir, testDB.Name()),
		)
		s.Require().NoError(result.Err, "can restore a database directory under a new name")
	}, "--db", testDB.Name())

	s.assertCappedAndPlainCollectionsRestored(restoredDB, before)
}

func (s *DumpRestoreSuite) testCappedRoundTripWithNSInclude() {
	testDB := s.database("capped_nsinclude")
	before := s.createCappedAndPlainCollections(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.NSIncludeOption, testDB.Name()+".*",
			dir,
		)
		s.Require().NoError(result.Err, "can restore with --nsInclude")
	}, "--db", testDB.Name())

	s.assertCappedAndPlainCollectionsRestored(testDB, before)
}

func (s *DumpRestoreSuite) testCappedRoundTripSingleColl() {
	testDB := s.database("capped_single_coll")
	before := s.createCappedAndPlainCollections(testDB)
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

	s.assertCappedCollectionRestored(restoredColl, before)
}

// cappedCollectionState records what the capped collection looked like before
// the dump, so the restored collection can be compared against it. The server
// rounds the requested capped size, so the options must come from the server
// rather than from the values that were requested.
type cappedCollectionState struct {
	docCount int64
	options  bson.D
}

// createCappedAndPlainCollections creates a plain collection with two secondary indexes and
// a capped collection that has already evicted documents.
func (s *DumpRestoreSuite) createCappedAndPlainCollections(
	testDB *mongo.Database,
) cappedCollectionState {
	const cappedSize = 1000

	// Far more documents than the capped collection can hold, so the server
	// evicts as it inserts and the resulting count is the cap itself.
	const cappedInsertCount = 1000

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
		Equal(true, optionValue(cappedOptions, "capped"), "the collection was created capped")

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
		"every index was created",
	)

	return cappedCollectionState{docCount: cappedCount, options: cappedOptions}
}

func (s *DumpRestoreSuite) assertCappedAndPlainCollectionsRestored(
	testDB *mongo.Database,
	before cappedCollectionState,
) {
	plainColl := testDB.Collection("plain")
	cappedColl := testDB.Collection("capped")

	s.Assert().EqualValues(1, s.docCount(plainColl), "the plain collection is restored")
	s.Assert().ElementsMatch(
		[]string{"_id_", "a_1", "b_1__id_-1"},
		s.indexNames(plainColl),
		"the plain collection's indexes are restored",
	)

	s.assertCappedCollectionRestored(cappedColl, before)

	// Only the capped fields are compared. Newer servers report additional
	// default options that mongodump does not carry in its metadata, so
	// comparing the whole options document breaks on server upgrades.
	restoredOptions := s.collectionOptions(testDB, cappedColl.Name())
	for _, field := range []string{"capped", "size", "max"} {
		s.Assert().EqualValues(
			optionValue(before.options, field),
			optionValue(restoredOptions, field),
			"the %#q option survives the round trip",
			field,
		)
	}
}

// assertCappedCollectionRestored checks the capped collection, which is
// restored under its own name in most of these tests and under a new one when a
// single collection is restored.
func (s *DumpRestoreSuite) assertCappedCollectionRestored(
	coll *mongo.Collection,
	before cappedCollectionState,
) {
	s.Assert().EqualValues(
		before.docCount,
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
	s.assertCappedEvictionWorks(coll, before.docCount)
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

// TestRestoreExtendedJSONMetadata restores metadata of the shape that very old
// versions of mongodump wrote: legacy extended JSON types for numeric values in
// both the collection options and the index specs, v1 indexes, and an "ns"
// field. Modern mongodump cannot produce such a dump, so the metadata is
// marshaled as canonical extended JSON here, which wraps every numeric value the
// way those old dumps did.
func (s *DumpRestoreSuite) TestRestoreExtendedJSONMetadata() {
	const collName = "changelog"
	const extJSONMetadataCappedSize = 10_000_000

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
	s.Require().NoError(err, "can marshal the metadata")

	s.Require().NoError(
		os.WriteFile(filepath.Join(dbDir, collName+".metadata.json"), metadata, 0644),
		"can write the metadata file",
	)
	s.writeBSONFile(
		filepath.Join(dbDir, collName+".bson"),
		bson.D{{"_id", 1}, {"pos", bson.A{1, 2}}},
		bson.D{{"_id", 2}, {"pos", bson.A{3, 4}}},
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

const optionsDocCount = 50

var optionsCollNames = []string{"capped", "validated", "plain"}

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
	s.createCollectionsWithOptions(testDB)

	cappedOptions := s.collectionOptions(testDB, "capped")
	validatedOptions := s.collectionOptions(testDB, "validated")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore the dump")
	}, "--db", testDB.Name())

	s.assertOptionsCollectionsRestored(testDB)

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
	s.createCollectionsWithOptions(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(mongorestore.NoOptionsRestoreOption, dir)
		s.Require().NoError(result.Err, "can restore with --noOptionsRestore")
	}, "--db", testDB.Name())

	s.assertOptionsCollectionsRestored(testDB)

	for _, collName := range optionsCollNames {
		s.assertOptionsStripped(testDB, collName)
	}
}

// assertOptionsStripped checks that none of the options the dump set survive.
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
	s.createCollectionsWithOptions(testDB)

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
		optionsDocCount,
		s.docCount(testDB.Collection("capped")),
		"the documents are restored",
	)
	s.assertOptionsStripped(testDB, "capped")
}

// createCollectionsWithOptions creates a capped collection, a collection with a
// validator, and a collection with no options at all, each holding the same
// documents.
func (s *DumpRestoreSuite) createCollectionsWithOptions(testDB *mongo.Database) {
	// Large enough to hold every document, so a document count check cannot pass
	// just because the collection is capped.
	const optionsCappedSize = 4096

	s.createCollection(
		testDB,
		"capped",
		options.CreateCollection().SetCapped(true).SetSizeInBytes(optionsCappedSize),
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
		"the capped collection really is capped",
	)
	s.Require().EqualValues(
		optionsCappedSize,
		optionValue(cappedOptions, "size"),
		"the capped collection has the requested capped size",
	)
	s.Require().NotNil(
		s.collectionOption(testDB, "validated", "validator"),
		"the validated collection really has a validator",
	)

	docs := make([]any, optionsDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", i}, {"phone", "abc"}}
	}

	for _, collName := range optionsCollNames {
		_, err := testDB.Collection(collName).InsertMany(s.Context(), docs)
		s.Require().NoError(err, "can insert into %#q", collName)
	}
}

func (s *DumpRestoreSuite) assertOptionsCollectionsRestored(testDB *mongo.Database) {
	for _, collName := range optionsCollNames {
		s.Assert().EqualValues(
			optionsDocCount,
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
	validationCollName     = "validated"
	validationDocCount     = 1000
	validationValidCount   = validationDocCount / 2
	validationInvalidCount = validationDocCount - validationValidCount
)

var documentValidator = bson.D{{"baz", bson.D{{"$exists", true}}}}

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
	s.insertValidationDocuments(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)
		s.createValidatedCollection(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "restoring against a validator still succeeds")
		s.Assert().EqualValues(
			validationInvalidCount,
			result.Failures,
			"mongorestore reports the rejected documents as failures",
		)
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationValidCount,
		s.docCount(testDB.Collection(validationCollName)),
		"only the valid documents are restored",
	)
}

func (s *DumpRestoreSuite) testBypassDocumentValidation() {
	testDB := s.database("validation_bypass")
	s.insertValidationDocuments(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)
		s.createValidatedCollection(testDB)

		result := s.runRestore(mongorestore.BypassDocumentValidationOption, dir)
		s.Require().NoError(result.Err, "can restore with --bypassDocumentValidation")
		s.Assert().EqualValues(0, result.Failures, "no documents are rejected")
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationDocCount,
		s.docCount(testDB.Collection(validationCollName)),
		"every document is restored when validation is bypassed",
	)
}

// testRestoredValidatorDropsInvalidDocs checks that the validator itself
// round-trips through the dump's metadata: restoring into an empty database
// recreates it, and the invalid documents in the same dump are then rejected.
func (s *DumpRestoreSuite) testRestoredValidatorDropsInvalidDocs() {
	testDB := s.database("validation_restored")
	s.insertValidationDocuments(testDB)
	s.addValidatorToCollection(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump that carries a validator")
		s.Assert().EqualValues(
			validationInvalidCount,
			result.Failures,
			"mongorestore reports the rejected documents as failures",
		)
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationValidCount,
		s.docCount(testDB.Collection(validationCollName)),
		"only the valid documents are restored",
	)
	s.assertRestoredValidatorMatches(testDB)
}

// testBypassRestoredValidator checks the combination of the two previous cases:
// mongorestore creates the collection with the validator from the dump's
// metadata and must still bypass that validator for its own inserts.
func (s *DumpRestoreSuite) testBypassRestoredValidator() {
	testDB := s.database("validation_bypass_restored")
	s.insertValidationDocuments(testDB)
	s.addValidatorToCollection(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(mongorestore.BypassDocumentValidationOption, dir)
		s.Require().NoError(result.Err, "can restore a dump that carries a validator")
		s.Assert().EqualValues(0, result.Failures, "no documents are rejected")
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		validationDocCount,
		s.docCount(testDB.Collection(validationCollName)),
		"every document is restored when validation is bypassed",
	)
	s.assertRestoredValidatorMatches(testDB)
}

func (s *DumpRestoreSuite) assertRestoredValidatorMatches(testDB *mongo.Database) {
	s.Assert().Equal(
		documentValidator,
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
	s.insertValidationDocuments(testDB)

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

// insertValidationDocuments inserts documents into validationCollName, of which
// only half satisfy the validator that the tests later apply. No validator is in
// place yet, so all of them are inserted.
func (s *DumpRestoreSuite) insertValidationDocuments(testDB *mongo.Database) {
	docs := make([]any, validationDocCount)
	for i := range docs {
		doc := bson.D{{"_id", i}, {"num", i + 1}}
		if i%2 == 0 {
			doc = append(doc, bson.E{"baz", i})
		}
		docs[i] = doc
	}

	_, err := testDB.Collection(validationCollName).InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the validation documents")
}

func (s *DumpRestoreSuite) createValidatedCollection(testDB *mongo.Database) {
	s.createCollection(
		testDB,
		validationCollName,
		options.CreateCollection().SetValidator(documentValidator),
	)
}

func (s *DumpRestoreSuite) addValidatorToCollection(testDB *mongo.Database) {
	res := testDB.RunCommand(s.Context(), bson.D{
		{"collMod", validationCollName},
		{"validator", documentValidator},
	})
	s.Require().NoError(res.Err(), "can add a validator to an existing collection")
}

// TestSystemProfileIsNotRestored checks that mongorestore skips a
// system.profile collection in a dump. mongodump does not write one for an
// ordinary database, because it skips <db>.system.*, so the dump has to be built
// by hand to reach the restore-side check at all. admin is the exception, where
// only system.keys is skipped.
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
		"can write the system.profile metadata file",
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
