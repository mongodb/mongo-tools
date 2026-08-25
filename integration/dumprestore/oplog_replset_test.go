package dumprestore

import (
	"fmt"
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestOplogReplayFromLocalOplogRS dumps local.oplog.rs with a $gt timestamp
// query, capturing only the ops written after a checkpoint, then replays that
// dump with --oplogReplay --oplogFile and checks exactly those ops are applied
// and the ops preceding the checkpoint are not.
func (s *DumpRestoreSuite) TestOplogReplayFromLocalOplogRS() {
	// local.oplog.rs only exists on a replica set. The suite as a whole gates on
	// the integration test type, so this case needs a gate of its own.
	testtype.SkipUnlessTestType(s.T(), testtype.ReplSetTestType)

	const collName = "coll"

	session, err := testutil.GetBareSession(s.T())
	s.Require().NoError(err, "can connect to the server")

	testDB := session.Database(uniqueDBName())
	coll := testDB.Collection(collName)
	ns := testDB.Name() + "." + collName

	// The two batch sizes differ on purpose. A replay that honors the checkpoint,
	// one that ignores it and replays everything, and one that replays the wrong
	// side of it produce three different document counts (secondBatch,
	// firstBatch+secondBatch, and firstBatch); equal sizes would make the last
	// two indistinguishable from success.
	const (
		firstBatch  = 5
		secondBatch = 7
	)

	// The first batch precedes the checkpoint and must not be replayed.
	for i := range firstBatch {
		_, err := coll.InsertOne(s.Context(), bson.D{{"_id", i}, {"batch", 1}})
		s.Require().NoError(err, "inserting the first batch")
	}

	checkpoint := s.latestOplogTimestamp()

	// The second batch follows the checkpoint and must be replayed.
	for i := range secondBatch {
		_, err := coll.InsertOne(s.Context(), bson.D{{"_id", 100 + i}, {"batch", 2}})
		s.Require().NoError(err, "inserting the second batch")
	}

	dumpDir, cleanupDump := testutil.MakeTempDir(s.T())
	defer cleanupDump()

	s.runMongodumpWithArgs(
		"--out", dumpDir,
		"--db", "local",
		"--collection", "oplog.rs",
		"--query", fmt.Sprintf(
			`{"ts": {"$gt": {"$timestamp": {"t": %d, "i": %d}}}, "ns": %q, "op": "i"}`,
			checkpoint.T,
			checkpoint.I,
			ns,
		),
	)

	s.Require().NoError(coll.Drop(s.Context()), "dropping the collection before the replay")
	s.Require().NoError(
		testDB.CreateCollection(s.Context(), collName),
		"recreating the collection before the replay",
	)

	// --oplogFile names the oplog to replay, so the target directory only has to
	// exist and hold nothing else.
	emptyDir, cleanupEmpty := testutil.MakeTempDir(s.T())
	defer cleanupEmpty()

	restore, err := getRestoreWithArgs(
		mongorestore.OplogReplayOption,
		mongorestore.OplogFileOption, filepath.Join(dumpDir, "local", "oplog.rs.bson"),
		mongorestore.DirectoryOption, emptyDir,
	)
	s.Require().NoError(err, "can build mongorestore")
	defer restore.Close()

	s.Require().NoError(restore.Restore().Err, "replaying the dumped oplog succeeds")

	total, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err, "counting the replayed documents")
	s.Assert().EqualValues(
		secondBatch,
		total,
		"only the ops after the checkpoint are replayed",
	)

	fromFirstBatch, err := coll.CountDocuments(s.Context(), bson.D{{"batch", 1}})
	s.Require().NoError(err, "counting the first-batch documents")
	s.Assert().EqualValues(
		0,
		fromFirstBatch,
		"the ops before the checkpoint are not replayed",
	)
}

// latestOplogTimestamp returns the ts of the newest entry in local.oplog.rs,
// which is the checkpoint the dump query filters on.
func (s *DumpRestoreSuite) latestOplogTimestamp() bson.Timestamp {
	session, err := testutil.GetBareSession(s.T())
	s.Require().NoError(err, "can connect to the server")

	var entry struct {
		TS bson.Timestamp `bson:"ts"`
	}
	err = session.Database("local").Collection("oplog.rs").
		FindOne(
			s.Context(),
			bson.D{},
			mopt.FindOne().SetSort(bson.D{{"$natural", -1}}),
		).
		Decode(&entry)
	s.Require().NoError(err, "reading the newest oplog entry")

	return entry.TS
}
