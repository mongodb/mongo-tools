package dumprestore

import (
	"path/filepath"
	"strconv"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// TestDumpFromSecondaryRestoreToPrimary dumps from a secondary and restores to
// the primary, which is how a dump is taken without putting its read load on the
// node taking writes.
func (s *DumpRestoreSuite) TestDumpFromSecondaryRestoreToPrimary() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	const collName = "bar"

	testDB := s.database("dump_from_secondary")
	coll := s.fullSetWriteCollection(testDB, collName)
	s.insertNamespacedDocs(coll)

	dir := s.T().TempDir()
	s.dumpWithArgs(
		s.DirectToolArgs(s.SecondaryHost()),
		"--out", dir,
		"--db", testDB.Name(),
		"--collection", collName,
	)

	s.dropCollection(coll)

	s.restoreWithArgs(
		s.DirectToolArgs(s.PrimaryHost()),
		"--db", testDB.Name(),
		"--collection", collName,
		filepath.Join(dir, testDB.Name(), collName+".bson"),
	)

	s.assertDocsCameFrom(testDB.Collection(collName), namespaceOf(coll))
}

// TestRestoreToSecondaryFails checks that a restore addressed at a secondary
// fails rather than reporting success for writes the secondary cannot take.
func (s *DumpRestoreSuite) TestRestoreToSecondaryFails() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	const collName = "bar"

	testDB := s.database("restore_to_secondary")
	coll := testDB.Collection(collName)
	s.insertNamespacedDocs(coll)

	dir := s.T().TempDir()
	s.dumpWithArgs(
		s.DirectToolArgs(s.PrimaryHost()),
		"--out", dir,
		"--db", testDB.Name(),
		"--collection", collName,
	)

	args := append(s.DirectToolArgs(s.SecondaryHost()),
		"--db", testDB.Name(),
		"--collection", collName,
		filepath.Join(dir, testDB.Name(), collName+".bson"),
	)
	opts, err := mongorestore.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongorestore options")

	restore, err := mongorestore.New(opts)
	s.Require().NoError(err, "mongorestore can connect to the secondary")
	defer restore.Close()

	restoreErr := restore.Restore().Err
	s.Require().NotNil(restoreErr, "mongorestore fails when it is pointed at a secondary")
	s.Require().Regexp(
		sharedsuite.NotWritablePrimary,
		restoreErr.Error(),
		"mongorestore fails because the secondary will not take the writes",
	)
}

// TestRestoreWithWriteConcernForWholeSet restores with a --writeConcern covering
// every node of the set, which does not return until all of them have the writes.
// The documents are counted on a secondary right afterwards with nothing waiting
// for replication, so a write concern that was not honored shows up as a short
// count.
//
// dumprestore10.js passed --writeConcern 2 against a two-node set, where that was
// the whole set. Here the number comes from the set's own membership: on a
// three-node set, w:2 is satisfied by the primary and whichever secondary happens
// to reply first, so the secondary this test reads from could lag and fail it.
func (s *DumpRestoreSuite) TestRestoreWithWriteConcernForWholeSet() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	const collName = "bar"

	testDB := s.database("restore_write_concern")
	coll := s.fullSetWriteCollection(testDB, collName)
	s.insertNamespacedDocs(coll)

	dir := s.T().TempDir()
	s.dumpWithArgs(
		s.DirectToolArgs(s.PrimaryHost()),
		"--out", dir,
		"--db", testDB.Name(),
		"--collection", collName,
	)

	secondaryHosts := s.SecondaryHosts()
	secondary := s.DirectClient(secondaryHosts[0])
	defer func() {
		s.Require().NoError(
			secondary.Disconnect(s.Context()),
			"can disconnect from the secondary",
		)
	}()
	secondaryColl := secondary.Database(testDB.Name()).Collection(collName)

	s.dropCollection(coll)
	// Without this, documents the drop has not yet reached would let the count
	// below pass on the fixture rather than on anything the restore wrote.
	s.Require().Zero(
		s.countOnSecondary(secondaryColl),
		"the drop reaches the secondary before the restore runs",
	)

	s.restoreWithArgs(
		s.DirectToolArgs(s.PrimaryHost()),
		"--writeConcern", strconv.Itoa(1+len(secondaryHosts)),
		"--db", testDB.Name(),
		"--collection", collName,
		filepath.Join(dir, testDB.Name(), collName+".bson"),
	)

	s.Assert().EqualValues(
		namespaceDocCount,
		s.countOnSecondary(secondaryColl),
		"the secondary has every restored document as soon as the restore returns",
	)
}

// fullSetWriteCollection returns the collection with a write concern covering
// every node of the set, so that a test reading from one particular secondary
// does not have to wait for replication itself. A majority would not do: on a
// three-node set it is satisfied by the other secondary.
func (s *DumpRestoreSuite) fullSetWriteCollection(
	testDB *mongo.Database,
	collName string,
) *mongo.Collection {
	nodes := 1 + len(s.SecondaryHosts())

	return testDB.Collection(
		collName,
		mopt.Collection().SetWriteConcern(&writeconcern.WriteConcern{W: nodes}),
	)
}

func (s *DumpRestoreSuite) dumpWithArgs(hostArgs []string, dumpArgs ...string) {
	opts, err := mongodump.ParseOptions(append(hostArgs, dumpArgs...), "", "")
	s.Require().NoError(err, "can parse the mongodump options")

	dump := &mongodump.MongoDump{
		ToolOptions:   opts.ToolOptions,
		InputOptions:  opts.InputOptions,
		OutputOptions: opts.OutputOptions,
	}
	s.Require().NoError(dump.Init(), "mongodump can connect")
	s.Require().NoError(dump.Dump(), "mongodump succeeds")
}

func (s *DumpRestoreSuite) restoreWithArgs(hostArgs []string, restoreArgs ...string) {
	opts, err := mongorestore.ParseOptions(append(hostArgs, restoreArgs...), "", "")
	s.Require().NoError(err, "can parse the mongorestore options")

	restore, err := mongorestore.New(opts)
	s.Require().NoError(err, "mongorestore can connect")
	defer restore.Close()

	s.Require().NoError(restore.Restore().Err, "mongorestore succeeds")
}

func (s *DumpRestoreSuite) countOnSecondary(coll *mongo.Collection) int64 {
	count, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err, "can count the documents in %#q on the secondary", coll.Name())

	return count
}
