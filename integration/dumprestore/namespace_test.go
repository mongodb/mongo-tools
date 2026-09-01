package dumprestore

import (
	"fmt"
	"os"
	"path/filepath"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// namespaceDocCount is small on purpose: these tests care about which namespace a document lands
// in, not about volume, and every case pays for a mongodump subprocess.
const namespaceDocCount = 50

// TestRestoreIntoDifferentCollection checks that --db and --collection can send a single
// collection's bson file to a collection of another name, another database, or both.
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

// TestRestoreIntoDifferentDB checks the two ways a restore can redirect a whole database: --db
// pointed at a database directory, and a --nsFrom/--nsTo mapping that rewrites both halves of the
// namespace.
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

// TestRestoreMultipleDBs checks that a dump spanning two databases restores each one's data back
// where it came from, including a collection whose name the two databases share.
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

	// The collection name is shared, so a restore that confused the two databases would still
	// produce the right counts. The document _id includes their source namespace, which is what
	// makes this check meaningful.
	s.assertDocsCameFrom(dbOne.Collection(sharedCollName), dbOne.Name()+"."+sharedCollName)
	s.assertDocsCameFrom(dbTwo.Collection(sharedCollName), dbTwo.Name()+"."+sharedCollName)
}

// TestPartialRestore checks that restoring one database, or one collection, out of a dump that
// holds more leaves everything else absent rather than merely empty.
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

// TestRestoreIntoDifferentDBBuildsIndexes covers SERVER-2186: when a dump is restored into a
// database with a different name, the destination must get the indexes, and a same-named database
// that happens to exist must not be touched.
func (s *DumpRestoreSuite) TestRestoreIntoDifferentDBBuildsIndexes() {
	const collName = "different_dbs_builds_indexes"

	sourceDB := s.database("different_db_indexes_source")
	destDB := s.database("different_db_indexes_dest")

	sourceColl := sourceDB.Collection(collName)
	s.insertNamespacedDocs(sourceColl)
	_, err := sourceColl.Indexes().CreateOne(s.Context(), mongo.IndexModel{Keys: bson.D{{"x", 1}}})
	s.Require().NoError(err, "can create the source index")

	s.withBSONMongodump(func(dir string) {
		// Dropping only the collection leaves the source database in place, so the restore has a
		// same-named collection it could wrongly write to.
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

// requireInserted checks that the restore inserted a whole number of source
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

var namespaceSourceDBs = []struct {
	suffix   string
	collNums []string
}{
	{"1", []string{"1", "2", "3"}},
	{"2", []string{"1", "2", "3"}},
	{"3", []string{"3", "4"}},
}

type namespaceMappingCase struct {
	dir          string
	sourcePrefix string
	name         string
	filterArgs   []string

	// restoredCollNums lists the source collection numbers expected to survive the filter under
	// test. A number missing from a case that should have kept it is caught by the document-count
	// assertion, which compares against what mongorestore actually inserted.
	restoredCollNums mapset.Set[string]
}

// TestRestoreNamespaceMappings checks the namespace filters and rewrites against three source
// databases: --nsExclude, --nsInclude and --excludeCollectionsWithPrefix each combined with a
// mapping that folds every source database into one destination, and a pair of mappings that
// exchange two databases.
func (s *DumpRestoreSuite) TestRestoreNamespaceMappings() {
	const sourcePrefix = "dumprestore_ns_source"

	s.createNamespaceSourceDBs(sourcePrefix)

	// One set of source databases and one dump serve every case below. Each mongodump is a separate
	// process launch, and rebuilding these three databases per case was the most expensive thing in
	// the package.
	s.withMultiDBDump(namespaceSourceDBNames(sourcePrefix), func(dir string) {
		// The filtering cases assert that collections are absent afterwards, which would hold just
		// as well if the dump were empty.
		s.requireNamespaceSourceDBsDumped(dir, sourcePrefix)
		s.dropNamespaceSourceDBs(sourcePrefix)

		s.Run("nsExclude drops the matching collections", func() {
			s.restoreAndAssertNamespaceMapping(namespaceMappingCase{
				dir:              dir,
				sourcePrefix:     sourcePrefix,
				name:             "ns_exclude",
				filterArgs:       []string{mongorestore.NSExcludeOption, "*.coll1"},
				restoredCollNums: mapset.NewSet("2", "3", "4"),
			})
		})

		s.Run("nsInclude keeps only the matching collections", func() {
			s.restoreAndAssertNamespaceMapping(namespaceMappingCase{
				dir:              dir,
				sourcePrefix:     sourcePrefix,
				name:             "ns_include",
				filterArgs:       []string{mongorestore.NSIncludeOption, "*.coll1"},
				restoredCollNums: mapset.NewSet("1"),
			})
		})

		s.Run("excludeCollectionsWithPrefix drops by prefix", func() {
			s.restoreAndAssertNamespaceMapping(namespaceMappingCase{
				dir:          dir,
				sourcePrefix: sourcePrefix,
				name:         "ns_prefix",
				filterArgs: []string{
					mongorestore.ExcludedCollectionPrefixesOption, "coll",
				},
				restoredCollNums: mapset.NewSet[string](),
			})
		})

		// Runs last: unlike the cases above it restores back over the source namespaces rather than
		// into a separate destination.
		s.Run("two mappings can swap a pair of databases", func() {
			s.restoreAndAssertNamespaceSwap(dir, sourcePrefix)
		})
	})
}

// createNamespaceSourceDBs builds three databases whose documents and indexes both encode the
// namespace they were created in, so a restore that maps a namespace to the wrong place is visible
// in the data rather than only in the counts.
func (s *DumpRestoreSuite) createNamespaceSourceDBs(sourcePrefix string) {
	client := s.Client()

	for _, sourceDB := range namespaceSourceDBs {
		testDB := client.Database(sourcePrefix + sourceDB.suffix)
		for _, collNum := range sourceDB.collNums {
			coll := testDB.Collection("coll" + collNum)
			s.insertNamespacedDocs(coll)

			_, err := coll.Indexes().CreateOne(
				s.Context(),
				mongo.IndexModel{Keys: bson.D{{namespaceOf(coll), 1}}},
			)
			s.Require().NoError(err, "can create an index on %#q", namespaceOf(coll))
		}
	}
}

// withMultiDBDump dumps several databases into one dump root and runs the test case against
// it. mongodump accepts a single --db, so a dump spanning several databases has to be built one
// database at a time; dumping everything instead would sweep in admin and config.
func (s *DumpRestoreSuite) withMultiDBDump(dbNames []string, testCase func(string)) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()

	for _, dbName := range dbNames {
		s.runMongodumpWithArgs("--out", dir, "--db", dbName)
	}

	testCase(dir)
}

func namespaceSourceDBNames(sourcePrefix string) []string {
	names := make([]string, 0, len(namespaceSourceDBs))
	for _, sourceDB := range namespaceSourceDBs {
		names = append(names, sourcePrefix+sourceDB.suffix)
	}

	return names
}

// requireNamespaceSourceDBsDumped is what keeps a case that expects nothing to be restored from
// passing when the dump itself was empty. Such a case asserts on a restored count of zero, which is
// also what a skipped dump produces.
func (s *DumpRestoreSuite) requireNamespaceSourceDBsDumped(dir, sourcePrefix string) {
	for _, sourceDB := range namespaceSourceDBs {
		for _, collNum := range sourceDB.collNums {
			bsonFile := filepath.Join(
				dir,
				sourcePrefix+sourceDB.suffix,
				"coll"+collNum+".bson",
			)
			info, err := os.Stat(bsonFile)
			s.Require().NoError(err, "the dump contains %#q", bsonFile)
			s.Require().Positive(info.Size(), "%#q holds the dumped documents", bsonFile)
		}
	}
}

func (s *DumpRestoreSuite) dropNamespaceSourceDBs(sourcePrefix string) {
	client := s.Client()
	for _, dbName := range namespaceSourceDBNames(sourcePrefix) {
		s.dropDB(client.Database(dbName))
	}
}

// restoreAndAssertNamespaceMapping restores the three source databases through a filter plus a
// mapping that folds every source database into one destination, encoding the source database and
// collection numbers into the destination collection name so each restored namespace can be traced
// back to its origin.
func (s *DumpRestoreSuite) restoreAndAssertNamespaceMapping(mappingCase namespaceMappingCase) {
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
		for _, collNum := range sourceDB.collNums {
			if mappingCase.restoredCollNums.Contains(collNum) {
				wantRestored++
			}
		}
	}

	s.Require().EqualValues(
		wantRestored*namespaceDocCount,
		result.Successes,
		"mongorestore inserted exactly the documents the filter lets through",
	)

	// Nothing may land back in the source namespaces either, which is the other way a filtered-out
	// collection could reappear.
	for _, dbName := range namespaceSourceDBNames(mappingCase.sourcePrefix) {
		s.Assert().Empty(
			s.collectionNames(s.Client().Database(dbName)),
			"nothing is restored into the source database %#q",
			dbName,
		)
	}

	for _, sourceDB := range namespaceSourceDBs {
		for _, collNum := range sourceDB.collNums {
			collName := "coll" + collNum
			restoredName := fmt.Sprintf("coll_%s_%s", sourceDB.suffix, collNum)

			if !mappingCase.restoredCollNums.Contains(collNum) {
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

// restoreAndAssertNamespaceSwap restores the source databases back over themselves with two
// mappings that exchange the first two databases. Both mappings have to be resolved against the
// original namespaces: applying them in sequence would send everything to one database.
func (s *DumpRestoreSuite) restoreAndAssertNamespaceSwap(dir, sourcePrefix string) {
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
		namespaceSourceDocCount(),
		result.Successes,
		"every source document is restored",
	)

	client := s.Client()
	for _, collNum := range namespaceSourceDBs[0].collNums {
		collName := "coll" + collNum
		s.assertDocsCameFrom(client.Database(dbOne).Collection(collName), dbTwo+"."+collName)
		s.assertSourceIndexRestored(client.Database(dbOne).Collection(collName), dbTwo, collName)
		s.assertDocsCameFrom(client.Database(dbTwo).Collection(collName), dbOne+"."+collName)
		s.assertSourceIndexRestored(client.Database(dbTwo).Collection(collName), dbOne, collName)
	}

	// The third database has no mapping, so it must land back where it started.
	for _, collNum := range namespaceSourceDBs[2].collNums {
		collName := "coll" + collNum
		s.assertDocsCameFrom(client.Database(dbThree).Collection(collName), dbThree+"."+collName)
		s.assertSourceIndexRestored(
			client.Database(dbThree).Collection(collName),
			dbThree,
			collName,
		)
	}
}

// insertNamespacedDocs writes documents whose _id records the namespace they were inserted into, so
// a document found in the wrong place after a restore still says where it came from.
func (s *DumpRestoreSuite) insertNamespacedDocs(coll *mongo.Collection) {
	_, err := coll.InsertMany(s.Context(), s.namespacedDocs(namespaceOf(coll)))
	s.Require().NoError(err, "can insert documents into %#q", namespaceOf(coll))
}

func namespaceOf(coll *mongo.Collection) string {
	return coll.Database().Name() + "." + coll.Name()
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

	wantIDs := lo.Map(
		s.namespacedDocs(sourceNS),
		func(doc bson.D, _ int) string {
			id, err := bsonutil.FindStringValueByKey("_id", &doc)
			s.Require().NoError(err, "finding _id in BSON doc")
			return id
		},
	)

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

// assertSourceIndexRestored checks that the index built over a field named for the source namespace
// followed the collection to its new name. The key is checked, not just the index name: a restore
// that recreated the index under the right name but the wrong key would otherwise pass.
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

// namespaceSourceDocCount is the total number of documents across every collection of every source
// database.
func namespaceSourceDocCount() int {
	total := 0
	for _, sourceDB := range namespaceSourceDBs {
		total += len(sourceDB.collNums) * namespaceDocCount
	}

	return total
}

func (s *DumpRestoreSuite) namespacedDocs(ns string) []bson.D {
	docs := make([]bson.D, namespaceDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", fmt.Sprintf("%d_%s", i, ns)}}
	}
	return docs
}
