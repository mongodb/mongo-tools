package dumprestore

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testutil"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// specialCollNames hold characters that have to be escaped to become filenames
// in a directory-format dump: a slash would otherwise create a subdirectory, and
// a percent is the escape character itself.
var specialCollNames = []string{"coll/foo", "coll%bar", "coll%2Fbaz"}

// TestRestoreSpecialCollectionNames round-trips collections whose names cannot
// be used verbatim as filenames. The archive format carries the name as data and
// is covered by TestPipedDumpRestore; this covers the directory format, where
// the name has to survive being escaped into a filename and unescaped again.
// coll%2Fbaz is a double-escaping guard: if % ever stopped being escaped on the
// dump side, it would unescape back to coll/baz and restore into the wrong
// namespace.
func (s *DumpRestoreSuite) TestRestoreSpecialCollectionNames() {
	testDB := s.database("special_coll_names")
	for _, collName := range specialCollNames {
		s.insertNamespacedDocs(testDB.Collection(collName))
	}

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore collections with escaped names")
		s.requireInserted(result, len(specialCollNames))
	}, "--db", testDB.Name())

	for _, collName := range specialCollNames {
		s.assertDocsCameFrom(
			testDB.Collection(collName),
			testDB.Name()+"."+collName,
		)
	}

	s.Assert().ElementsMatch(
		specialCollNames,
		s.collectionNames(testDB),
		"every collection is restored under its original name",
	)
}

// TestArchiveThroughStdout runs a real mongodump process with --archive and
// pipes its standard output into a real mongorestore process reading from
// standard input. TestPipedDumpRestore covers the archive format itself, but it
// connects the two in-process through an io.Pipe, so the CLI plumbing this test
// exercises -- --archive with no value selecting stdout and stdin, and a binary
// archive surviving a real pipe -- is not covered anywhere else.
//
// The restore writes back over the same namespaces with --drop, which is what
// makes the escaped collection names worth including here: they have to survive
// the archive as well as a drop and recreate.
func (s *DumpRestoreSuite) TestArchiveThroughStdout() {
	testDB := s.database("archive_stdout")
	for _, collName := range specialCollNames {
		s.insertNamespacedDocs(testDB.Collection(collName))
	}

	dump := exec.Command(
		"go",
		append(
			[]string{"run", filepath.Join("..", "..", "mongodump", "main")},
			append(testopts.GetBareArgs(), "--archive", "--db", testDB.Name())...,
		)...,
	)
	restore := exec.Command(
		"go",
		append(
			[]string{"run", filepath.Join("..", "..", "mongorestore", "main")},
			append(testopts.GetBareArgs(), "--archive", "--drop")...,
		)...,
	)

	archive, err := dump.StdoutPipe()
	s.Require().NoError(err, "can open the dump's standard output")
	restore.Stdin = archive

	var dumpErr, restoreErr bytes.Buffer
	dump.Stderr = &dumpErr
	restore.Stderr = &restoreErr

	s.Require().NoError(restore.Start(), "can start mongorestore")
	s.Require().NoError(dump.Run(), "mongodump succeeds (stderr: %s)", dumpErr.String())
	s.Require().NoError(restore.Wait(), "mongorestore succeeds (stderr: %s)", restoreErr.String())

	// The restore writes back over the namespaces it read from, so the documents
	// being present afterwards does not by itself show that anything came
	// through the pipe. The count mongorestore reports does.
	s.Require().Regexp(
		fmt.Sprintf(
			`\b%d document\(s\) restored successfully`,
			len(specialCollNames)*namespaceDocCount,
		),
		restoreErr.String(),
		"mongorestore restored every document the archive carried",
	)

	for _, collName := range specialCollNames {
		s.assertDocsCameFrom(
			testDB.Collection(collName),
			testDB.Name()+"."+collName,
		)
	}
}

// TestRestoreDumpWithSymlinks restores a dump directory in which a database
// directory and a collection's bson file are both symlinks, and which also
// contains a symlink to a plain file at the top level. The last one is the point
// of the test: a symlinked regular file must not be walked as though it were a
// database directory.
func (s *DumpRestoreSuite) TestRestoreDumpWithSymlinks() {
	if runtime.GOOS == "windows" {
		s.T().Skip("Skipping test because it creates symlinks, which Windows restricts")
	}

	linkedDB := s.database("symlinks_linked_db")
	directDB := s.database("symlinks_direct_db")

	root, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	dumpDir := filepath.Join(root, "dump")

	// The real contents live outside the dump directory, so the only way
	// mongorestore can reach them is by following the links.
	outside := filepath.Join(root, "outside")
	s.Require().NoError(os.MkdirAll(outside, 0755), "can create the link target directory")

	linkedDBDir := filepath.Join(outside, "linked_db")
	s.Require().NoError(os.MkdirAll(linkedDBDir, 0755), "can create the linked database directory")
	s.writeNamespacedBSONFile(
		filepath.Join(linkedDBDir, "data.bson"),
		linkedDB.Name()+".data",
	)

	directDBDir := filepath.Join(dumpDir, directDB.Name())
	s.Require().NoError(os.MkdirAll(directDBDir, 0755), "can create the dump directory")

	linkedCollFile := filepath.Join(outside, "linked_collection.bson")
	s.writeNamespacedBSONFile(linkedCollFile, directDB.Name()+".data")

	s.Require().NoError(
		os.Symlink(linkedDBDir, filepath.Join(dumpDir, linkedDB.Name())),
		"can link a database directory into the dump",
	)
	s.Require().NoError(
		os.Symlink(linkedCollFile, filepath.Join(directDBDir, "data.bson")),
		"can link a collection file into the dump",
	)
	s.Require().NoError(
		os.Symlink(linkedCollFile, filepath.Join(dumpDir, "not_a_dir")),
		"can link a plain file into the top of the dump",
	)

	result := s.runRestore(dumpDir)
	s.Require().NoError(result.Err, "can restore a dump containing symlinks")
	s.requireInserted(result, 2)

	s.assertDocsCameFrom(linkedDB.Collection("data"), linkedDB.Name()+".data")
	s.assertDocsCameFrom(directDB.Collection("data"), directDB.Name()+".data")

	// The insert count above is what proves the symlinked plain file was
	// skipped: had mongorestore walked it as a database directory it would
	// either have errored or restored a third collection's worth of documents.
	s.Assert().NotContains(
		s.databaseNames(),
		"not_a_dir",
		"a symlink to a plain file is not treated as a database directory",
	)
}

// writeNamespacedBSONFile writes the same documents insertNamespacedDocs would
// have inserted, so the restored data can be checked with assertDocsCameFrom.
func (s *DumpRestoreSuite) writeNamespacedBSONFile(path, sourceNS string) {
	docs := make([]bson.D, namespaceDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", namespacedID(i, sourceNS)}}
	}

	s.writeBSONFile(path, docs...)
}

func (s *DumpRestoreSuite) databaseNames() []string {
	names, err := s.Client().ListDatabaseNames(s.Context(), bson.D{})
	s.Require().NoError(err, "can list the databases")

	return names
}
