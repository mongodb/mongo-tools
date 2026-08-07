package dumprestore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mongodb/mongo-tools/common/testutil"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestDumpDBFlag checks that --db narrows the dump to one database, both in what
// lands on disk and in what a later restore brings back. The sibling database
// holds a collection of the same name, so a dump that ignored --db would still
// produce a plausible-looking result.
func (s *DumpRestoreSuite) TestDumpDBFlag() {
	const collName = "bar"

	dumpedDB := s.database("db_flag_dumped")
	otherDB := s.database("db_flag_other")

	s.insertNamespacedDocs(dumpedDB.Collection(collName))
	s.insertNamespacedDocs(otherDB.Collection(collName))

	s.withBSONMongodump(func(dir string) {
		s.Assert().DirExists(
			filepath.Join(dir, dumpedDB.Name()),
			"the dump holds the database that was asked for",
		)
		s.Assert().NoDirExists(
			filepath.Join(dir, otherDB.Name()),
			"the dump does not hold the database that was not asked for",
		)

		s.dropDB(dumpedDB)
		s.dropDB(otherDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore the single-database dump")
		s.requireInserted(result, 1)
	}, "--db", dumpedDB.Name())

	s.assertDocsCameFrom(dumpedDB.Collection(collName), dumpedDB.Name()+"."+collName)
	s.Assert().Empty(
		s.collectionNames(otherDB),
		"the database that was never dumped stays empty",
	)
}

// TestDumpCollectionFlag checks that --collection narrows the dump to one
// collection. The sibling collection and a sibling database both hold data, so a
// dump that ignored --collection would still look plausible.
func (s *DumpRestoreSuite) TestDumpCollectionFlag() {
	const dumpedCollName = "bar"

	testDB := s.database("collection_flag")
	otherDB := s.database("collection_flag_other")

	s.insertNamespacedDocs(testDB.Collection(dumpedCollName))
	s.insertNamespacedDocs(testDB.Collection("sibling"))
	s.insertNamespacedDocs(otherDB.Collection(dumpedCollName))

	s.withBSONMongodump(func(dir string) {
		s.assertDumpHasCollections(
			dir,
			testDB.Name(),
			[]string{dumpedCollName},
			[]string{"sibling"},
		)
		s.Assert().NoDirExists(
			filepath.Join(dir, otherDB.Name()),
			"the dump does not reach into another database",
		)

		s.dropDB(testDB)
		s.dropDB(otherDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore the single-collection dump")
		s.requireInserted(result, 1)
	}, "--db", testDB.Name(), "--collection", dumpedCollName)

	s.assertDocsCameFrom(
		testDB.Collection(dumpedCollName),
		testDB.Name()+"."+dumpedCollName,
	)
	s.Assert().NotContains(
		s.collectionNames(testDB),
		"sibling",
		"the collection that was not dumped is not restored",
	)
	s.Assert().Empty(
		s.collectionNames(otherDB),
		"the database that was not dumped stays empty",
	)
}

// TestDumpExcludeFlags checks the exclusion flags: --excludeCollection, given
// once and repeated, and --excludeCollectionsWithPrefix. Each case leaves at
// least one collection in the dump, so an exclusion that dropped everything
// would not pass.
func (s *DumpRestoreSuite) TestDumpExcludeFlags() {
	s.Run("excludeCollection omits one collection", s.testDumpExcludeCollection)
	s.Run("excludeCollectionsWithPrefix omits a group", s.testDumpExcludeCollectionsWithPrefix)
	s.Run("both exclude flags apply together", s.testDumpExcludeBothFlags)
}

func (s *DumpRestoreSuite) testDumpExcludeCollection() {
	testDB := s.database("exclude_collection")
	for _, collName := range []string{"keep", "drop"} {
		s.insertNamespacedDocs(testDB.Collection(collName))
	}

	s.withBSONMongodump(func(dir string) {
		s.assertDumpHasCollections(dir, testDB.Name(), []string{"keep"}, []string{"drop"})

		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump with an excluded collection")
		s.requireInserted(result, 1)
	}, "--db", testDB.Name(), "--excludeCollection", "drop")

	s.assertDocsCameFrom(testDB.Collection("keep"), testDB.Name()+".keep")
	s.Assert().NotContains(
		s.collectionNames(testDB),
		"drop",
		"the excluded collection is not restored",
	)
}

func (s *DumpRestoreSuite) testDumpExcludeCollectionsWithPrefix() {
	testDB := s.database("exclude_prefix")
	// One victim has a dot in its name: dotted names are the interesting case
	// both for prefix matching and for escaping the name into a filename.
	for _, collName := range []string{"keep", "skipme", "skip.dotted"} {
		s.insertNamespacedDocs(testDB.Collection(collName))
	}

	s.withBSONMongodump(func(dir string) {
		s.assertDumpHasCollections(
			dir,
			testDB.Name(),
			[]string{"keep"},
			[]string{"skipme", "skip.dotted"},
		)

		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump with an excluded prefix")
		s.requireInserted(result, 1)
	}, "--db", testDB.Name(), "--excludeCollectionsWithPrefix", "skip")

	s.assertDocsCameFrom(testDB.Collection("keep"), testDB.Name()+".keep")
	for _, collName := range []string{"skipme", "skip.dotted"} {
		s.Assert().NotContains(
			s.collectionNames(testDB),
			collName,
			"the collection %#q matching the excluded prefix is not restored",
			collName,
		)
	}
}

// testDumpExcludeBothFlags covers the two exclude flags on one command line. The
// filtering logic with both lists populated is unit-tested in
// mongodump/prepare_test.go; what is untested elsewhere is that both flags are
// actually wired through from the command line.
func (s *DumpRestoreSuite) testDumpExcludeBothFlags() {
	testDB := s.database("exclude_both")
	for _, collName := range []string{"keep", "byname", "skipme"} {
		s.insertNamespacedDocs(testDB.Collection(collName))
	}

	s.withBSONMongodump(func(dir string) {
		s.assertDumpHasCollections(
			dir,
			testDB.Name(),
			[]string{"keep"},
			[]string{"byname", "skipme"},
		)

		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump with both exclude flags")
		s.requireInserted(result, 1)
	},
		"--db", testDB.Name(),
		"--excludeCollection", "byname",
		"--excludeCollectionsWithPrefix", "skip",
	)

	s.assertDocsCameFrom(testDB.Collection("keep"), testDB.Name()+".keep")
	for _, collName := range []string{"byname", "skipme"} {
		s.Assert().NotContains(
			s.collectionNames(testDB),
			collName,
			"the excluded collection %#q is not restored",
			collName,
		)
	}
}

// assertDumpHasCollections checks the dump directory itself rather than only the
// restored data, so an exclusion that merely failed to restore would not pass
// for an exclusion that never happened.
func (s *DumpRestoreSuite) assertDumpHasCollections(
	dir string,
	dbName string,
	wantPresent []string,
	wantAbsent []string,
) {
	for _, collName := range wantPresent {
		bsonPath := filepath.Join(dir, dbName, collName+".bson")

		info, err := os.Stat(bsonPath)
		s.Require().NoError(err, "the dump holds %#q", collName)
		s.Assert().Positive(info.Size(), "the dump of %#q holds its documents", collName)
	}

	for _, collName := range wantAbsent {
		s.Assert().NoFileExists(
			filepath.Join(dir, dbName, collName+".bson"),
			"the dump does not hold the excluded %#q",
			collName,
		)
	}
}

// TestDumpToStdout checks that --out - writes a single collection's bson to
// standard output. The option validation around it is covered by unit tests in
// the mongodump package; this covers that the bytes actually arrive.
func (s *DumpRestoreSuite) TestDumpToStdout() {
	const collName = "bar"

	testDB := s.database("out_stdout")
	s.insertNamespacedDocs(testDB.Collection(collName))

	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()

	bsonPath := filepath.Join(dir, "stdout.bson")
	bsonFile, err := os.Create(bsonPath)
	s.Require().NoError(err, "can create the file to capture standard output")

	defer bsonFile.Close()

	s.runMongodumpToWriter(
		bsonFile,
		"--out", "-",
		"--db", testDB.Name(),
		"--collection", collName,
	)
	s.Require().NoError(bsonFile.Close(), "can close the captured output")

	info, err := os.Stat(bsonPath)
	s.Require().NoError(err, "the captured output exists")
	s.Require().Positive(info.Size(), "mongodump wrote bson to standard output")

	s.dropDB(testDB)

	result := s.runRestore(
		"--db", testDB.Name(),
		"--collection", collName,
		bsonPath,
	)
	s.Require().NoError(result.Err, "can restore the bson captured from standard output")
	s.requireInserted(result, 1)

	s.assertDocsCameFrom(testDB.Collection(collName), testDB.Name()+"."+collName)
}

// TestDumpQuotedQueryArgument runs the real mongodump binary with a --query
// value containing spaces, and checks the query both survived the trip and was
// applied. Passing such a value through ParseOptions in a unit test proves
// nothing: the value is already a single argv entry by then, so there is nothing
// left that could split it.
func (s *DumpRestoreSuite) TestDumpQuotedQueryArgument() {
	const (
		collName = "coll"
		docCount = 10
		wantKept = 4
	)

	testDB := s.database("quoted_query")
	coll := testDB.Collection(collName)

	docs := make([]any, docCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", i}, {"a", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the fixture documents")

	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()

	s.runMongodumpWithArgs(
		"--out", dir,
		"--db", testDB.Name(),
		"--collection", collName,
		"--query", `{ "a": { "$gte": 6 } }`,
	)

	s.dropDB(testDB)

	result := s.runRestore(dir)
	s.Require().NoError(result.Err, "can restore the filtered dump")
	s.Assert().EqualValues(
		wantKept,
		result.Successes,
		"only the documents matching the quoted query were dumped",
	)
	s.Assert().EqualValues(wantKept, s.docCount(coll), "only the matching documents come back")
}

// runMongodumpToWriter runs mongodump with its standard output redirected, which
// the suite's own dump helper cannot do because it captures output for logging.
func (s *DumpRestoreSuite) runMongodumpToWriter(out *os.File, args ...string) {
	cmdArgs := []string{"run", filepath.Join("..", "..", "mongodump", "main")}
	cmdArgs = append(cmdArgs, testutil.GetBareArgs()...)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdout = out

	var stderr strings.Builder
	cmd.Stderr = &stderr

	s.Require().NoError(
		cmd.Run(),
		"can run mongodump with %v (stderr: %s)",
		args,
		stderr.String(),
	)
}
