package dumprestore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// namespaceDocCount is small on purpose: these tests care about which namespace
// a document lands in, not about volume, and every case pays for a mongodump
// subprocess.
const namespaceDocCount = 50

// TestRestoreIntoDifferentCollection checks that --db and --collection can send
// a single collection's bson file to a collection of another name, another
// database, or both.
func (s *DumpRestoreSuite) TestRestoreIntoDifferentCollection() {
	testDB := s.database("different_collection")
	sourceColl := testDB.Collection("sourceColl")
	s.insertNamespacedDocs(sourceColl)

	otherDB := s.database("different_collection_other")

	s.withBSONMongodumpForCollection(testDB.Name(), sourceColl.Name(), func(dir string) {
		bsonFile := filepath.Join(dir, testDB.Name(), sourceColl.Name()+".bson")
		sourceNS := namespaceOf(sourceColl)

		s.Run("a different collection in the same database", func() {
			s.restoreInto(testDB.Name(), "destColl", bsonFile)
			s.assertDocsCameFrom(testDB.Collection("destColl"), sourceNS)
		})

		s.Run("the same collection name in a different database", func() {
			s.restoreInto(otherDB.Name(), sourceColl.Name(), bsonFile)
			s.assertDocsCameFrom(otherDB.Collection(sourceColl.Name()), sourceNS)
		})

		s.Run("a different collection in a different database", func() {
			s.restoreInto(otherDB.Name(), "destColl", bsonFile)
			s.assertDocsCameFrom(otherDB.Collection("destColl"), sourceNS)
		})
	})
}

func (s *DumpRestoreSuite) restoreInto(dbName, collName, bsonFile string) {
	result := s.runRestore(
		mongorestore.DBOption, dbName,
		mongorestore.CollectionOption, collName,
		bsonFile,
	)
	s.Require().NoError(result.Err, "can restore into %s.%s", dbName, collName)
	s.Require().EqualValues(
		namespaceDocCount,
		result.Successes,
		"the restore into %s.%s inserted the dumped documents",
		dbName,
		collName,
	)
}

// TestRestoreIntoDifferentDB checks the two ways a restore can redirect a whole
// database: --db pointed at a database directory, and a --nsFrom/--nsTo mapping
// that rewrites both halves of the namespace.
func (s *DumpRestoreSuite) TestRestoreIntoDifferentDB() {
	collNames := []string{"coll1", "coll2"}

	sourceDB := s.database("different_db_source")
	for _, collName := range collNames {
		s.insertNamespacedDocs(sourceDB.Collection(collName))
	}

	destDB := s.database("different_db_dest")
	otherDestDB := s.database("different_db_otherdest")

	s.withBSONMongodump(func(dir string) {
		s.Run("--db redirects a database directory", func() {
			result := s.runRestore(
				mongorestore.DBOption, destDB.Name(),
				mongorestore.DirectoryOption, filepath.Join(dir, sourceDB.Name()),
			)
			s.Require().NoError(result.Err, "can restore a database directory under a new name")
			s.requireInserted(result, len(collNames))

			for _, collName := range collNames {
				s.assertDocsCameFrom(
					destDB.Collection(collName),
					sourceDB.Name()+"."+collName,
				)
			}
		})

		s.Run("--nsFrom and --nsTo rewrite both parts of the namespace", func() {
			result := s.runRestore(
				mongorestore.NSFromOption, "$db$.$collection$",
				mongorestore.NSToOption, otherDestDB.Name()+".$db$_$collection$",
				dir,
			)
			s.Require().NoError(result.Err, "can restore with a namespace mapping")
			s.requireInserted(result, len(collNames))

			for _, collName := range collNames {
				s.assertDocsCameFrom(
					otherDestDB.Collection(sourceDB.Name()+"_"+collName),
					sourceDB.Name()+"."+collName,
				)
			}
		})
	}, "--db", sourceDB.Name())
}

// TestRestoreMultipleDBs checks that a dump spanning two databases restores each
// one's data back where it came from, including a collection whose name the two
// databases share.
func (s *DumpRestoreSuite) TestRestoreMultipleDBs() {
	const sharedCollName = "bothColl"

	dbOne := s.database("multiple_dbs_one")
	dbTwo := s.database("multiple_dbs_two")

	s.insertNamespacedDocs(dbOne.Collection("dbOneColl"))
	s.insertNamespacedDocs(dbTwo.Collection("dbTwoColl"))
	s.insertNamespacedDocs(dbOne.Collection(sharedCollName))
	s.insertNamespacedDocs(dbTwo.Collection(sharedCollName))

	s.withMultiDBDump([]string{dbOne.Name(), dbTwo.Name()}, func(dir string) {
		s.dropDB(dbOne)
		s.dropDB(dbTwo)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump spanning two databases")
		s.requireInserted(result, 4)
	})

	s.assertDocsCameFrom(dbOne.Collection("dbOneColl"), dbOne.Name()+".dbOneColl")
	s.assertDocsCameFrom(dbTwo.Collection("dbTwoColl"), dbTwo.Name()+".dbTwoColl")

	// The collection name is shared, so a restore that confused the two
	// databases would still produce the right counts. The documents carry their
	// source namespace, which is what makes this check meaningful.
	s.assertDocsCameFrom(dbOne.Collection(sharedCollName), dbOne.Name()+"."+sharedCollName)
	s.assertDocsCameFrom(dbTwo.Collection(sharedCollName), dbTwo.Name()+"."+sharedCollName)
}

// TestPartialRestore checks that restoring one database, or one collection, out
// of a dump that holds more leaves everything else absent rather than merely
// empty.
func (s *DumpRestoreSuite) TestPartialRestore() {
	dbOne := s.database("partial_restore_one")
	dbTwo := s.database("partial_restore_two")

	s.insertNamespacedDocs(dbOne.Collection("collOne"))
	s.insertNamespacedDocs(dbOne.Collection("collTwo"))
	s.insertNamespacedDocs(dbTwo.Collection("collThree"))

	s.withMultiDBDump([]string{dbOne.Name(), dbTwo.Name()}, func(dir string) {
		s.dropDB(dbOne)
		s.dropDB(dbTwo)

		s.Run("one database out of a multi-database dump", func() {
			result := s.runRestore(
				mongorestore.DBOption, dbOne.Name(),
				mongorestore.DirectoryOption, filepath.Join(dir, dbOne.Name()),
			)
			s.Require().NoError(result.Err, "can restore a single database")
			s.requireInserted(result, 2)

			s.assertDocsCameFrom(dbOne.Collection("collOne"), dbOne.Name()+".collOne")
			s.assertDocsCameFrom(dbOne.Collection("collTwo"), dbOne.Name()+".collTwo")
			s.Assert().Empty(
				s.collectionNames(dbTwo),
				"the database that was not asked for is left alone",
			)
		})

		s.Run("one collection out of a multi-database dump", func() {
			s.dropDB(dbOne)

			result := s.runRestore(
				mongorestore.DBOption, dbOne.Name(),
				mongorestore.CollectionOption, "collTwo",
				filepath.Join(dir, dbOne.Name(), "collTwo.bson"),
			)
			s.Require().NoError(result.Err, "can restore a single collection")
			s.requireInserted(result, 1)

			s.assertDocsCameFrom(dbOne.Collection("collTwo"), dbOne.Name()+".collTwo")
			s.Assert().NotContains(
				s.collectionNames(dbOne),
				"collOne",
				"the sibling collection is not restored",
			)
			s.Assert().Empty(
				s.collectionNames(dbTwo),
				"the database that was not asked for is left alone",
			)
		})
	})
}

// TestRestoreIntoDifferentDBBuildsIndexes covers SERVER-2186: when a dump is
// restored into a database with a different name, the destination must get the
// indexes, and a same-named database that happens to exist must not be touched.
func (s *DumpRestoreSuite) TestRestoreIntoDifferentDBBuildsIndexes() {
	const collName = "dumprestore4"

	sourceDB := s.database("different_db_indexes_source")
	destDB := s.database("different_db_indexes_dest")

	sourceColl := sourceDB.Collection(collName)
	s.insertNamespacedDocs(sourceColl)
	_, err := sourceColl.Indexes().CreateOne(s.Context(), mongo.IndexModel{Keys: bson.D{{"x", 1}}})
	s.Require().NoError(err, "can create the source index")

	s.withBSONMongodump(func(dir string) {
		// Dropping only the collection leaves the source database in place, so
		// the restore has a same-named collection it could wrongly write to.
		s.dropCollection(sourceColl)

		result := s.runRestore(
			mongorestore.DBOption, destDB.Name(),
			mongorestore.DirectoryOption, filepath.Join(dir, sourceDB.Name()),
		)
		s.Require().NoError(result.Err, "can restore into a differently named database")
		s.requireInserted(result, 1)
	}, "--db", sourceDB.Name())

	s.assertDocsCameFrom(destDB.Collection(collName), sourceDB.Name()+"."+collName)
	s.Assert().ElementsMatch(
		[]string{"_id_", "x_1"},
		s.indexNames(destDB.Collection(collName)),
		"the destination gets the source's indexes",
	)

	s.Assert().NotContains(
		s.collectionNames(sourceDB),
		collName,
		"the source database is left alone",
	)
}

var namespaceSourceDBs = []struct {
	suffix    string
	collNames []string
}{
	{"1", []string{"coll1", "coll2", "coll3"}},
	{"2", []string{"coll1", "coll2", "coll3"}},
	{"3", []string{"coll3", "coll4"}},
}

// TestRestoreNamespaceMappings checks the namespace filters and rewrites against
// a three-database fixture: --nsExclude, --nsInclude and
// --excludeCollectionsWithPrefix each combined with a mapping that folds every
// source database into one destination, and a pair of mappings that exchange two
// databases.
func (s *DumpRestoreSuite) TestRestoreNamespaceMappings() {
	const sourcePrefix = "dumprestore_ns_source"

	s.createNamespaceFixture(sourcePrefix)

	// One fixture and one dump serve every case below. Each mongodump is a
	// separate process launch, and rebuilding this three-database fixture per
	// case was the most expensive thing in the package.
	s.withMultiDBDump(namespaceFixtureDBNames(sourcePrefix), func(dir string) {
		// The filtering cases assert that collections are absent afterwards,
		// which would hold just as well if the dump were empty.
		s.requireNamespaceFixtureDumped(dir, sourcePrefix)
		s.dropNamespaceFixture(sourcePrefix)

		s.Run("nsExclude drops the matching collections", func() {
			s.assertNamespaceMapping(namespaceMappingCase{
				dir:           dir,
				sourcePrefix:  sourcePrefix,
				name:          "ns_exclude",
				filterArgs:    []string{mongorestore.NSExcludeOption, "*.coll1"},
				shouldRestore: func(collNum string) bool { return collNum != "1" },
			})
		})

		s.Run("nsInclude keeps only the matching collections", func() {
			s.assertNamespaceMapping(namespaceMappingCase{
				dir:           dir,
				sourcePrefix:  sourcePrefix,
				name:          "ns_include",
				filterArgs:    []string{mongorestore.NSIncludeOption, "*.coll1"},
				shouldRestore: func(collNum string) bool { return collNum == "1" },
			})
		})

		s.Run("excludeCollectionsWithPrefix drops by prefix", func() {
			s.assertNamespaceMapping(namespaceMappingCase{
				dir:          dir,
				sourcePrefix: sourcePrefix,
				name:         "ns_prefix",
				filterArgs: []string{
					mongorestore.ExcludedCollectionPrefixesOption, "coll",
				},
				shouldRestore: func(string) bool { return false },
			})
		})

		// Runs last: unlike the cases above it restores back over the source
		// namespaces rather than into a separate destination.
		s.Run("two mappings can swap a pair of databases", func() {
			s.assertNamespaceSwap(dir, sourcePrefix)
		})
	})
}

type namespaceMappingCase struct {
	dir          string
	sourcePrefix string
	name         string
	filterArgs   []string

	// shouldRestore reports whether the given source collection number is
	// expected to survive the filter under test.
	shouldRestore func(collNum string) bool
}

// assertNamespaceMapping restores the three-database fixture through a filter
// plus a mapping that folds every source database into one destination,
// encoding the source database and collection numbers into the destination
// collection name so each restored namespace can be traced back to its origin.
func (s *DumpRestoreSuite) assertNamespaceMapping(mappingCase namespaceMappingCase) {
	destDB := s.database(mappingCase.name + "_dest")

	args := append([]string{}, mappingCase.filterArgs...)
	args = append(
		args,
		mongorestore.NSFromOption, mappingCase.sourcePrefix+"$db-num$.coll$coll-num$",
		mongorestore.NSToOption, destDB.Name()+".coll_$db-num$_$coll-num$",
		mappingCase.dir,
	)

	result := s.runRestore(args...)
	s.Require().NoError(result.Err, "can restore with a namespace mapping")

	wantRestored := 0
	for _, sourceDB := range namespaceSourceDBs {
		for _, collName := range sourceDB.collNames {
			if mappingCase.shouldRestore(strings.TrimPrefix(collName, "coll")) {
				wantRestored++
			}
		}
	}

	// Without this, a case that expects nothing to be restored would pass even
	// if mongorestore had skipped the dump entirely.
	s.Require().EqualValues(
		wantRestored*namespaceDocCount,
		result.Successes,
		"mongorestore inserted exactly the documents the filter lets through",
	)

	// Nothing may land back in the source namespaces either, which is the other
	// way a filtered-out collection could reappear.
	for _, dbName := range namespaceFixtureDBNames(mappingCase.sourcePrefix) {
		s.Assert().Empty(
			s.collectionNames(s.client().Database(dbName)),
			"nothing is restored into the source database %#q",
			dbName,
		)
	}

	for _, sourceDB := range namespaceSourceDBs {
		for _, collName := range sourceDB.collNames {
			collNum := strings.TrimPrefix(collName, "coll")
			restoredName := fmt.Sprintf("coll_%s_%s", sourceDB.suffix, collNum)

			if !mappingCase.shouldRestore(collNum) {
				s.Assert().NotContains(
					s.collectionNames(destDB),
					restoredName,
					"%#q is filtered out",
					restoredName,
				)

				continue
			}

			restoredColl := destDB.Collection(restoredName)
			sourceDBName := mappingCase.sourcePrefix + sourceDB.suffix
			s.assertDocsCameFrom(restoredColl, sourceDBName+"."+collName)
			s.assertSourceIndexRestored(restoredColl, sourceDBName, collName)
		}
	}
}

// assertNamespaceSwap restores the fixture back over itself with two mappings
// that exchange the first two databases. Both mappings have to be resolved
// against the original namespaces: applying them in sequence would send
// everything to one database.
func (s *DumpRestoreSuite) assertNamespaceSwap(dir, sourcePrefix string) {
	dbOne := sourcePrefix + "1"
	dbTwo := sourcePrefix + "2"
	dbThree := sourcePrefix + "3"

	result := s.runRestore(
		mongorestore.NSFromOption, dbOne+".*",
		mongorestore.NSToOption, dbTwo+".*",
		mongorestore.NSFromOption, dbTwo+".*",
		mongorestore.NSToOption, dbOne+".*",
		dir,
	)
	s.Require().NoError(result.Err, "can restore with two namespace mappings")
	s.Require().EqualValues(
		namespaceFixtureDocCount(),
		result.Successes,
		"every fixture document is restored",
	)

	client := s.client()
	for _, collName := range namespaceSourceDBs[0].collNames {
		s.assertDocsCameFrom(client.Database(dbOne).Collection(collName), dbTwo+"."+collName)
		s.assertDocsCameFrom(client.Database(dbTwo).Collection(collName), dbOne+"."+collName)
	}

	// The third database has no mapping, so it must land back where it started.
	for _, collName := range namespaceSourceDBs[2].collNames {
		s.assertDocsCameFrom(client.Database(dbThree).Collection(collName), dbThree+"."+collName)
	}
}

// createNamespaceFixture builds three databases whose documents and indexes both
// encode the namespace they were created in, so a restore that maps a namespace
// to the wrong place is visible in the data rather than only in the counts.
func (s *DumpRestoreSuite) createNamespaceFixture(sourcePrefix string) {
	client := s.client()

	for _, sourceDB := range namespaceSourceDBs {
		testDB := client.Database(sourcePrefix + sourceDB.suffix)
		for _, collName := range sourceDB.collNames {
			coll := testDB.Collection(collName)
			s.insertNamespacedDocs(coll)

			_, err := coll.Indexes().CreateOne(
				s.Context(),
				mongo.IndexModel{Keys: bson.D{{namespaceOf(coll), 1}}},
			)
			s.Require().NoError(err, "can create an index on %#q", namespaceOf(coll))
		}
	}
}

func (s *DumpRestoreSuite) requireNamespaceFixtureDumped(dir, sourcePrefix string) {
	for _, sourceDB := range namespaceSourceDBs {
		for _, collName := range sourceDB.collNames {
			bsonFile := filepath.Join(dir, sourcePrefix+sourceDB.suffix, collName+".bson")
			info, err := os.Stat(bsonFile)
			s.Require().NoError(err, "the dump contains %#q", bsonFile)
			s.Require().Positive(info.Size(), "%#q holds the dumped documents", bsonFile)
		}
	}
}

// namespaceFixtureDocCount is the total number of documents across every
// collection of the fixture.
func namespaceFixtureDocCount() int {
	total := 0
	for _, sourceDB := range namespaceSourceDBs {
		total += len(sourceDB.collNames) * namespaceDocCount
	}

	return total
}

func namespaceFixtureDBNames(sourcePrefix string) []string {
	names := make([]string, 0, len(namespaceSourceDBs))
	for _, sourceDB := range namespaceSourceDBs {
		names = append(names, sourcePrefix+sourceDB.suffix)
	}

	return names
}

func (s *DumpRestoreSuite) dropNamespaceFixture(sourcePrefix string) {
	client := s.client()
	for _, dbName := range namespaceFixtureDBNames(sourcePrefix) {
		s.dropDB(client.Database(dbName))
	}
}

// assertSourceIndexRestored checks that the index built over a field named for
// the source namespace followed the collection to its new name. The key is
// checked, not just the index name: a restore that recreated the index under the
// right name but the wrong key would otherwise pass.
func (s *DumpRestoreSuite) assertSourceIndexRestored(
	coll *mongo.Collection,
	sourceDBName string,
	sourceCollName string,
) {
	sourceNS := sourceDBName + "." + sourceCollName

	key := s.indexKey(coll, sourceNS+"_1")
	s.Require().Len(key, 1, "the index from %#q has a single key field", sourceNS)
	s.Assert().Equal(
		sourceNS,
		key[0].Key,
		"the index from %#q follows the collection to %#q",
		sourceNS,
		coll.Name(),
	)
	s.Assert().EqualValues(1, key[0].Value, "the index from %#q keeps its direction", sourceNS)
}

// insertNamespacedDocs writes documents whose _id records the namespace they
// were inserted into, so a document found in the wrong place after a restore
// still says where it came from.
func (s *DumpRestoreSuite) insertNamespacedDocs(coll *mongo.Collection) {
	docs := make([]any, namespaceDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", namespacedID(i, namespaceOf(coll))}}
	}

	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert documents into %#q", namespaceOf(coll))
}

func (s *DumpRestoreSuite) assertDocsCameFrom(coll *mongo.Collection, sourceNS string) {
	var restored []struct {
		ID string `bson:"_id"`
	}
	cursor, err := coll.Find(s.Context(), bson.D{})
	s.Require().NoError(err, "can read %#q", namespaceOf(coll))
	s.Require().NoError(cursor.All(s.Context(), &restored), "can decode %#q", namespaceOf(coll))

	s.Require().Len(
		restored,
		namespaceDocCount,
		"%#q holds every document from %#q",
		namespaceOf(coll),
		sourceNS,
	)

	wantIDs := make([]string, namespaceDocCount)
	for i := range wantIDs {
		wantIDs[i] = namespacedID(i, sourceNS)
	}

	gotIDs := make([]string, 0, len(restored))
	for _, doc := range restored {
		gotIDs = append(gotIDs, doc.ID)
	}

	s.Assert().ElementsMatch(
		wantIDs,
		gotIDs,
		"every document in %#q came from %#q",
		namespaceOf(coll),
		sourceNS,
	)
}

// namespacedID builds the _id of a fixture document. Encoding the source
// namespace into the _id is what lets a restore that sends data to the wrong
// namespace be detected by content rather than only by document counts.
func namespacedID(i int, sourceNS string) string {
	return fmt.Sprintf("%d_%s", i, sourceNS)
}

// withMultiDBDump dumps several databases into one dump root and runs the test
// case against it. mongodump accepts a single --db, so a dump spanning several
// databases has to be built one database at a time; dumping everything instead
// would sweep in admin and config.
func (s *DumpRestoreSuite) withMultiDBDump(dbNames []string, testCase func(string)) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()

	for _, dbName := range dbNames {
		s.runMongodumpWithArgs("--out", dir, "--db", dbName)
	}

	testCase(dir)
}

// requireInserted checks that the restore inserted a whole number of fixture
// collections. mongorestore returns a nil error for a restore that found nothing
// to do, so without this a test could pass on a restore that never ran.
func (s *DumpRestoreSuite) requireInserted(result mongorestore.Result, collCount int) {
	s.Require().EqualValues(
		collCount*namespaceDocCount,
		result.Successes,
		"the restore inserted the documents of %d collections",
		collCount,
	)
}

// client returns a client for reaching databases these tests name themselves,
// rather than through the per-test database helper.
func (s *DumpRestoreSuite) client() *mongo.Client {
	session, err := testutil.GetBareSession()
	s.Require().NoError(err, "can connect to the server")

	return session
}

func namespaceOf(coll *mongo.Collection) string {
	return coll.Database().Name() + "." + coll.Name()
}
