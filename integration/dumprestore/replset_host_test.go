package dumprestore

import (
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/mongodb/mongo-tools/mongorestore"
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

	s.dumpOverReplicaSetHost(hostArgs, dir, testDB.Name(), collName)
	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection before restoring it")

	s.restoreOverReplicaSetHost(
		hostArgs,
		filepath.Join(dir, testDB.Name(), collName+".bson"),
		testDB.Name(),
		collName,
	)

	s.Assert().EqualValues(
		countBefore,
		s.docCount(coll),
		"every document comes back from a dump and restore addressed to the replica set",
	)
}

func (s *DumpRestoreSuite) dumpOverReplicaSetHost(hostArgs []string, dir, dbName, collName string) {
	args := append(append([]string{}, hostArgs...),
		"--out", dir,
		"--db", dbName,
		"--collection", collName,
	)
	opts, err := mongodump.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongodump options")

	dump := &mongodump.MongoDump{
		ToolOptions:   opts.ToolOptions,
		InputOptions:  opts.InputOptions,
		OutputOptions: opts.OutputOptions,
	}
	s.Require().NoError(dump.Init(), "mongodump can connect to the replica set")
	s.Require().NoError(dump.Dump(), "mongodump succeeds against the replica set")
}

func (s *DumpRestoreSuite) restoreOverReplicaSetHost(
	hostArgs []string,
	bsonFile, dbName, collName string,
) {
	args := append(append([]string{}, hostArgs...),
		"--db", dbName,
		"--collection", collName,
		bsonFile,
	)
	opts, err := mongorestore.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongorestore options")

	restore, err := mongorestore.New(opts)
	s.Require().NoError(err, "mongorestore can connect to the replica set")
	defer restore.Close()

	s.Require().NoError(restore.Restore().Err, "mongorestore succeeds against the replica set")
}
