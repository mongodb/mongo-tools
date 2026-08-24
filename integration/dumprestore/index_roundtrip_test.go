package dumprestore

import (
	"path/filepath"
	"strconv"

	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestIndexRoundTrip checks that every kind of index survives a dump and restore
// with its full specification intact, whether the restore reads the dump
// directory or the collection's bson file directly.
func (s *DumpRestoreSuite) TestIndexRoundTrip() {
	s.Run("restore from the dump directory", s.testIndexRoundTripFromDumpDir)
	s.Run("restore from the bson file", s.testIndexRoundTripFromBSONFile)
}

func (s *DumpRestoreSuite) testIndexRoundTripFromDumpDir() {
	testDB := s.database("indexes_dump_dir")
	coll := s.createCollectionWithEveryIndexKind(testDB)
	specsBefore := s.indexSpecs(coll)

	s.withBSONMongodump(func(dir string) {
		s.dropCollection(coll)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore from the dump directory")
	}, "--db", testDB.Name())

	s.assertEveryIndexKindRestored(coll, specsBefore)
}

func (s *DumpRestoreSuite) testIndexRoundTripFromBSONFile() {
	testDB := s.database("indexes_bson_file")
	coll := s.createCollectionWithEveryIndexKind(testDB)
	specsBefore := s.indexSpecs(coll)

	s.withBSONMongodump(func(dir string) {
		s.dropCollection(coll)

		result := s.runRestore(filepath.Join(dir, testDB.Name(), coll.Name()+".bson"))
		s.Require().NoError(result.Err, "can restore from the bson file")
	}, "--db", testDB.Name())

	s.assertEveryIndexKindRestored(coll, specsBefore)
}

const (
	// Each of three kinds of document is inserted this many times.
	indexDocsPerKind = 5
	indexDocCount    = 3 * indexDocsPerKind
)

// createCollectionWithEveryIndexKind creates one collection carrying an index of every kind
// worth round-tripping: simple, sparse and unique, compound, compound with
// int64 key values, multikey (via array data), text with a non-default
// language, and 2dsphere.
func (s *DumpRestoreSuite) createCollectionWithEveryIndexKind(
	testDB *mongo.Database,
) *mongo.Collection {
	ctx := s.Context()
	coll := testDB.Collection("coll")

	_, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{"a", 1}}},
		{Keys: bson.D{{"b", 1}}, Options: options.Index().SetSparse(true).SetUnique(true)},
		{Keys: bson.D{{"a", 1}, {"b", -1}}},
		{Keys: bson.D{{"b", int64(1)}, {"a", int64(1)}}},
		{Keys: bson.D{{"listField", 1}}},
		{
			Keys:    bson.D{{"textField", "text"}},
			Options: options.Index().SetDefaultLanguage("spanish"),
		},
		{Keys: bson.D{{"geoField", "2dsphere"}}},
	})
	s.Require().NoError(err, "can create every index")

	docs := make([]any, 0, indexDocCount)
	for i := range indexDocsPerKind {
		docs = append(
			docs,
			bson.D{{"a", i}, {"b", i + 1}, {"listField", bson.A{i, i + 1}}},
			bson.D{{"textField", "hola " + strconv.Itoa(i)}},
			bson.D{{"geoField", bson.D{
				{"type", "Point"},
				{"coordinates", bson.A{i, i + 1}},
			}}},
		)
	}
	_, err = coll.InsertMany(ctx, docs)
	s.Require().NoError(err, "can insert every document")

	s.Require().EqualValues(
		indexDocCount,
		s.docCount(coll),
		"every document was inserted",
	)
	// The seven indexes above, plus the _id index.
	const indexCount = 8

	specs := s.indexSpecs(coll)
	s.Require().Len(specs, indexCount, "every index was created")
	s.requireDistinctiveIndexPropertiesPresent(specs)

	return coll
}

// requireDistinctiveIndexPropertiesPresent checks that the properties these
// indexes exist to exercise really made it into the server's specs. Without
// this, a server that normalized int64 index keys to double, or that stopped
// reporting the text language, would turn the before/after spec comparison into
// a comparison of two identically uninteresting documents.
func (s *DumpRestoreSuite) requireDistinctiveIndexPropertiesPresent(specs []bson.D) {
	specsByName := s.indexSpecsByName(specs)

	compoundKey, ok := optionValue(specsByName["b_1_a_1"], "key").(bson.D)
	s.Require().True(ok, "the int64-keyed compound index reports a key document")
	for _, elem := range compoundKey {
		s.Require().IsType(
			int64(0),
			elem.Value,
			"the compound index key %#q keeps its int64 type",
			elem.Key,
		)
	}

	s.Require().Equal(
		"spanish",
		optionValue(specsByName["textField_text"], "default_language"),
		"the text index really uses a non-default language",
	)
}

func (s *DumpRestoreSuite) assertEveryIndexKindRestored(
	coll *mongo.Collection,
	specsBefore []bson.D,
) {
	s.Assert().EqualValues(indexDocCount, s.docCount(coll), "the documents are restored")
	s.assertIndexSpecsMatch(specsBefore, s.indexSpecs(coll), coll.Name())
}

// assertIndexSpecsMatch compares the whole spec of every index against what the
// server reported before the dump, field by field.
//
// The fields of a spec are compared as a set rather than as an ordered document.
// mongorestore holds the index options in a map (idx.IndexDocument.Options), so
// the order it sends them to createIndexes is arbitrary, and a server that
// reports back the order an index was created with — 4.2 does, later versions
// normalize it — makes the restored spec differ from the original by field order
// alone. The order the server does act on, that of the key and
// partialFilterExpression documents, is nested inside a field's value and so is
// still compared exactly.
func (s *DumpRestoreSuite) assertIndexSpecsMatch(
	specsBefore []bson.D,
	specsAfter []bson.D,
	collName string,
) {
	byName := s.indexSpecsByName(specsAfter)
	for name, specBefore := range s.indexSpecsByName(specsBefore) {
		specAfter, ok := byName[name]
		if !s.Assert().True(ok, "the index %#q on %#q is restored", name, collName) {
			continue
		}

		s.Assert().ElementsMatch(
			specBefore,
			specAfter,
			"the index %#q on %#q is restored with an identical spec",
			name,
			collName,
		)
	}

	s.Assert().Len(
		byName,
		len(specsBefore),
		"no index is restored on %#q that was not dumped",
		collName,
	)
}

func (s *DumpRestoreSuite) indexSpecsByName(specs []bson.D) map[string]bson.D {
	specsByName := make(map[string]bson.D, len(specs))
	for _, spec := range specs {
		name, ok := optionValue(spec, "name").(string)
		s.Require().True(ok, "the index spec reports a name")
		specsByName[name] = spec
	}

	return specsByName
}

// indexSpecs returns the complete spec of every index on the collection.
// ListSpecifications omits fields such as the index version and the text index
// options, so the specs are listed directly.
func (s *DumpRestoreSuite) indexSpecs(coll *mongo.Collection) []bson.D {
	var specs []bson.D
	s.Require().NoError(
		listIndexes(s.Context(), coll, &specs),
		"can list the index specs on %#q",
		coll.Name(),
	)

	return specs
}

// TestIndexVersionRoundTrip checks which version a restored index ends up at:
// --keepIndexVersion carries the dumped version through, without it the server
// applies its own default, and a legacy version in the dump is converted.
func (s *DumpRestoreSuite) TestIndexVersionRoundTrip() {
	s.Run("keepIndexVersion preserves the index version", s.testKeepIndexVersion)
	s.Run("the server default version is used otherwise", s.testDefaultIndexVersion)
}

func (s *DumpRestoreSuite) testKeepIndexVersion() {
	testDB := s.database("keep_index_version")
	s.createCollectionsWithIDIndexVersions(testDB)

	versionsBefore := s.idIndexVersions(testDB)
	s.Require().EqualValues(1, versionsBefore["v1coll"], "the v1 _id index was created")
	s.Require().EqualValues(2, versionsBefore["v2coll"], "the v2 _id index was created")
	specsBefore := s.indexSpecsByCollection(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(mongorestore.KeepIndexVersionOption, dir)
		s.Require().NoError(result.Err, "can restore with --keepIndexVersion")
	}, "--db", testDB.Name())

	s.assertIndexVersionCollectionsRestored(testDB)
	s.Assert().Equal(
		versionsBefore,
		s.idIndexVersions(testDB),
		"--keepIndexVersion restores each _id index at its original version",
	)

	// The secondary indexes are covered here rather than by an explicit version
	// assertion, because it's not certain the server actually honors the
	// requested v1/v2 version on a secondary index. Comparing the whole spec
	// checks the version the server actually chose, whatever that is.
	for collName, specs := range specsBefore {
		s.assertIndexSpecsMatch(specs, s.indexSpecs(testDB.Collection(collName)), collName)
	}
}

func (s *DumpRestoreSuite) testDefaultIndexVersion() {
	testDB := s.database("default_index_version")
	s.createCollectionsWithIDIndexVersions(testDB)

	s.Require().EqualValues(
		1,
		s.idIndexVersions(testDB)["v1coll"],
		"the v1 _id index was created",
	)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore without --keepIndexVersion")
	}, "--db", testDB.Name())

	s.assertIndexVersionCollectionsRestored(testDB)
	s.Assert().Equal(
		map[string]int32{"v1coll": 2, "v2coll": 2},
		s.idIndexVersions(testDB),
		"without --keepIndexVersion every _id index gets the server's default version",
	)
}

// indexVersionColls are the collections the index-version tests create, each
// with an _id index at the given version.
var indexVersionColls = []struct {
	name    string
	version int
}{
	{"v1coll", 1},
	{"v2coll", 2},
}

// createCollectionsWithIDIndexVersions creates two collections whose _id indexes differ
// only in their index version. The version can only be set through the create
// command's idIndex argument, so the secondary index below requests the same
// version explicitly; whatever version the server actually assigns it, the
// tests check that its spec survives a round trip.
func (s *DumpRestoreSuite) createCollectionsWithIDIndexVersions(testDB *mongo.Database) {
	for _, versionColl := range indexVersionColls {
		res := testDB.RunCommand(s.Context(), bson.D{
			{"create", versionColl.name},
			{"idIndex", bson.D{
				{"v", versionColl.version},
				{"key", bson.D{{"_id", 1}}},
				{"name", "_id_"},
			}},
		})
		s.Require().NoError(
			res.Err(),
			"can create %#q with a v%d _id index",
			versionColl.name,
			versionColl.version,
		)

		coll := testDB.Collection(versionColl.name)
		_, err := coll.Indexes().CreateOne(s.Context(), mongo.IndexModel{
			Keys:    bson.D{{"a", 1}},
			Options: options.Index().SetVersion(int32(versionColl.version)),
		})
		s.Require().NoError(err, "can create a secondary index on %#q", versionColl.name)

		_, err = coll.InsertOne(s.Context(), bson.D{{"a", 123}})
		s.Require().NoError(err, "can insert into %#q", versionColl.name)
	}
}

func (s *DumpRestoreSuite) assertIndexVersionCollectionsRestored(testDB *mongo.Database) {
	for _, versionColl := range indexVersionColls {
		coll := testDB.Collection(versionColl.name)
		s.Assert().EqualValues(1, s.docCount(coll), "%#q keeps its document", versionColl.name)
		s.Assert().ElementsMatch(
			[]string{"_id_", "a_1"},
			s.indexNames(coll),
			"%#q keeps both of its indexes",
			versionColl.name,
		)
	}
}

func (s *DumpRestoreSuite) idIndexVersions(testDB *mongo.Database) map[string]int32 {
	versions := map[string]int32{}
	for _, versionColl := range indexVersionColls {
		for _, spec := range s.indexSpecs(testDB.Collection(versionColl.name)) {
			if optionValue(spec, "name") == "_id_" {
				version, ok := optionValue(spec, "v").(int32)
				s.Require().True(ok, "the _id index of %#q reports a version", versionColl.name)
				versions[versionColl.name] = version
			}
		}
	}

	return versions
}

func (s *DumpRestoreSuite) indexSpecsByCollection(testDB *mongo.Database) map[string][]bson.D {
	specs := make(map[string][]bson.D, len(indexVersionColls))
	for _, versionColl := range indexVersionColls {
		specs[versionColl.name] = s.indexSpecs(testDB.Collection(versionColl.name))
	}

	return specs
}

// TestLegacySystemIndexes restores a dump in the pre-2.6 layout, where index
// specs live in a system.indexes.bson file instead of a per-collection metadata
// file. mongorestore falls back to that file when a database directory has no
// metadata, and converts the legacy v1 specs it finds there. Unlike the other
// cases here, this dump has to be built by hand, because no supported server can
// produce that layout or a v1 secondary index.
func (s *DumpRestoreSuite) TestLegacySystemIndexes() {
	const collName = "foo"

	testDB := s.database("legacy_system_indexes")
	_, dbDir := s.newDumpDir(testDB.Name())

	s.writeBSONFile(
		filepath.Join(dbDir, collName+".bson"),
		bson.D{{"_id", 1}, {"a", 2.0}},
	)
	// The namespaces deliberately name a different database than the restore
	// target, because mongorestore has to take the target from --db rather than
	// from the spec.
	s.writeBSONFile(
		filepath.Join(dbDir, "system.indexes.bson"),
		bson.D{
			{"ns", "test." + collName},
			{"key", bson.D{{"_id", 1}}},
			{"name", "_id_"},
			{"v", 1},
		},
		bson.D{
			{"ns", "test." + collName},
			{"key", bson.D{{"a", 1.0}}},
			{"name", "a_1"},
			{"v", 1},
		},
	)

	result := s.runRestore(
		mongorestore.DBOption, testDB.Name(),
		mongorestore.DirectoryOption, dbDir,
	)
	s.Require().NoError(result.Err, "can restore a dump that uses system.indexes")

	coll := testDB.Collection(collName)
	s.Assert().EqualValues(1, s.docCount(coll), "the document is restored")
	s.Assert().ElementsMatch(
		[]string{"_id_", "a_1"},
		s.indexNames(coll),
		"both legacy index specs are created",
	)

	for _, spec := range s.indexSpecs(coll) {
		s.Assert().EqualValues(
			2,
			optionValue(spec, "v"),
			"the legacy v1 index %#q is converted to the current version",
			optionValue(spec, "name"),
		)
	}
}

// TestOrderedPartialIndex round-trips an index whose partialFilterExpression has
// many fields. The expression is a document, so its field order has to survive
// the trip through the dump's metadata file: reordering it would produce an index
// the server treats as different from the one that was dumped.
func (s *DumpRestoreSuite) TestOrderedPartialIndex() {
	const indexName = "a_1_partial"

	// Deliberately not in sorted order. Sorting is the most likely way for a
	// document to come back reordered, so an ascending list would match its own
	// corruption and the test would pass either way.
	partialFilterFieldOrder := []string{"a7", "a2", "a9", "a0", "a5", "a1", "a8", "a3", "a6", "a4"}

	testDB := s.database("ordered_partial_index")
	coll := testDB.Collection("foo")

	filter := bson.D{}
	for _, field := range partialFilterFieldOrder {
		filter = append(filter, bson.E{field, bson.D{{"$gt", 0}}})
	}

	_, err := coll.Indexes().CreateOne(s.Context(), mongo.IndexModel{
		Keys:    bson.D{{"a", 1}},
		Options: options.Index().SetName(indexName).SetPartialFilterExpression(filter),
	})
	s.Require().NoError(err, "can create an index with a partialFilterExpression")

	_, err = coll.InsertOne(s.Context(), bson.D{{"a", 1}})
	s.Require().NoError(err, "can insert a document")

	s.Require().Equal(
		partialFilterFieldOrder,
		s.partialFilterFields(coll, indexName),
		"the created index reports its filter fields in the order they were given",
	)

	s.withBSONMongodump(func(dir string) {
		s.dropCollection(coll)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore an index with a partialFilterExpression")
	}, "--db", testDB.Name())

	s.Assert().Equal(
		partialFilterFieldOrder,
		s.partialFilterFields(coll, indexName),
		"the partialFilterExpression keeps its field order through the round trip",
	)
}

func (s *DumpRestoreSuite) partialFilterFields(
	coll *mongo.Collection,
	indexName string,
) []string {
	filter := s.indexPartialFilter(coll, indexName)

	fields := make([]string, 0, len(filter))
	for _, elem := range filter {
		fields = append(fields, elem.Key)
	}

	return fields
}

func (s *DumpRestoreSuite) indexPartialFilter(
	coll *mongo.Collection,
	indexName string,
) bson.D {
	cursor, err := coll.Indexes().List(s.Context())
	s.Require().NoError(err, "can list indexes on %#q", coll.Name())

	var indexes []struct {
		Name                    string `bson:"name"`
		PartialFilterExpression bson.D `bson:"partialFilterExpression"`
	}
	s.Require().NoError(cursor.All(s.Context(), &indexes), "can read the index specs")

	for _, index := range indexes {
		if index.Name == indexName {
			return index.PartialFilterExpression
		}
	}

	s.Require().Failf("index not found", "the index %#q exists on %#q", indexName, coll.Name())

	return nil
}
