package dumprestore

import (
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/testtype"
)

// TestDumpRestoreOverReplicaSetHost dumps and restores through a
// "<setName>/<seedlist>" host argument rather than a single host, which is how
// the tools are pointed at a replica set without a connection string.
//
// The dump and restore are scoped to one collection where tool_replset.js dumped
// and restored the whole deployment, because the rest of this suite's state lives
// on the same server.
func (s *DumpRestoreSuite) TestDumpRestoreOverReplicaSetHost() {
	testtype.SkipUnlessTestType(s.T(), testtype.ReplSetTestType)

	const collName = "bar"

	testDB := s.database("replset_host")
	coll := testDB.Collection(collName)
	s.insertNamespacedDocs(coll)
	countBefore := s.docCount(coll)
	s.Require().Positive(countBefore, "the collection holds documents before the dump")

	hostArgs := s.ReplicaSetToolArgs()
	dir := s.T().TempDir()

	s.dumpWithArgs(
		hostArgs,
		"--out", dir,
		"--db", testDB.Name(),
		"--collection", collName,
	)
	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection before restoring it")

	s.restoreWithArgs(
		hostArgs,
		"--db", testDB.Name(),
		"--collection", collName,
		filepath.Join(dir, testDB.Name(), collName+".bson"),
	)

	s.Assert().EqualValues(
		countBefore,
		s.docCount(coll),
		"every document comes back from a dump and restore addressed to the replica set",
	)
}
