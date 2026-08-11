package dumprestore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestDumpOplogSnapshotDuringWrites checks the point-in-time guarantee --oplog
// makes about a dump taken while writes are in flight: the collection data plus
// the captured oplog together account for every document that existed when the
// dump finished. A dump without --oplog cannot promise that, because documents
// inserted after its cursor passed them are in neither half.
//
// The guarantee is checked against the two halves directly rather than by
// restoring with --oplogReplay. mongorestore rejects --oplogReplay alongside
// --nsInclude, and --oplog is only accepted on a full dump, so a replaying
// restore would have to write the whole cluster back -- including admin and
// config -- to test one collection. What replay would produce is exactly the
// union asserted here, and TestOplogReplayFromLocalOplogRS covers the replay
// machinery itself.
func (s *DumpRestoreSuite) TestDumpOplogSnapshotDuringWrites() {
	// The oplog only exists on a replica set. The suite as a whole gates on the
	// integration test type, so this case needs a gate of its own.
	testtype.SkipUnlessTestType(s.T(), testtype.ReplSetTestType)

	const collName = "bar"

	testDB := s.database("oplog_snapshot")
	coll := testDB.Collection(collName)

	// Dropping the database does not clear the server's oplog, so earlier runs of
	// this test wrote entries for this same namespace that are still there. The
	// lookup of the concurrent inserts below starts from here to leave them out.
	since := s.latestOplogTimestamp()

	s.insertNamespacedDocs(coll)
	preexistingIDs := s.documentIDs(coll)
	countBefore := s.docCount(coll)
	s.Require().Positive(countBefore, "the collection holds documents before the dump")

	ns := testDB.Name() + "." + collName

	var wantIDs, oplogIDs []string
	s.withConcurrentInserts(coll, func(stopInserts func()) {
		s.withBSONMongodump(func(dir string) {
			stopInserts()

			s.Require().Greater(
				s.docCount(coll),
				countBefore,
				"the concurrent inserts landed while the dump was running",
			)

			oplogPath := filepath.Join(dir, "oplog.bson")
			oplogIDs = s.insertedIDsFromOplogFile(oplogPath, ns)

			// The dump's point in time is where its captured oplog ends, which is
			// earlier than now: mongodump takes that timestamp and then still has
			// to write the oplog out and exit, and the inserts ran until it did.
			//
			// The pre-existing documents are added separately because they predate the
			// dump either way, and because they went in as one batch, which the
			// server records as a single applyOps entry rather than an insert op
			// each.
			wantIDs = lo.Union(preexistingIDs, s.insertedIDsFromServerOplog(
				ns,
				since,
				s.lastTimestampInOplogFile(oplogPath),
			))

			s.dropDB(testDB)

			result := s.runRestore(mongorestore.NSIncludeOption, testDB.Name()+".*", dir)
			s.Require().NoError(result.Err, "can restore the collection half of the dump")
		}, "--oplog")
	})

	// Without this the union below could hold whatever the data half happened to
	// hold, and would still match if --oplog had captured nothing at all.
	s.Require().NotEmpty(
		oplogIDs,
		"the captured oplog holds inserts, so the dump really did span concurrent writes",
	)

	s.Assert().ElementsMatch(
		wantIDs,
		lo.Union(s.documentIDs(coll), oplogIDs),
		"the dumped data and the captured oplog together account for exactly the documents "+
			"that existed at the dump's point in time",
	)
}

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

// stringIDDoc decodes just the _id of a document. Every document these tests
// insert has a string _id, so the ids can be compared as sets.
type stringIDDoc struct {
	ID string `bson:"_id"`
}

// documentIDs returns the _ids in coll.
func (s *DumpRestoreSuite) documentIDs(coll *mongo.Collection) []string {
	cursor, err := coll.Find(s.Context(), bson.D{})
	s.Require().NoError(err, "can read %#q", coll.Name())

	var docs []stringIDDoc
	s.Require().
		NoError(cursor.All(s.Context(), &docs), "can decode the documents in %#q", coll.Name())

	return lo.Map(docs, func(doc stringIDDoc, _ int) string {
		return doc.ID
	})
}

// insertedIDsFromOplogFile returns the _ids of the insert ops in a dumped
// oplog.bson that target ns, which are the documents mongorestore --oplogReplay
// would add on top of the dumped collection data.
func (s *DumpRestoreSuite) insertedIDsFromOplogFile(path, ns string) []string {
	var ids []string
	s.eachOpInFile(path, func(op db.Oplog) {
		if op.Operation != "i" || op.Namespace != ns {
			return
		}

		id, err := bsonutil.FindValueByKey("_id", &op.Object)
		s.Require().NoError(err, "an insert op in the captured oplog has an _id")

		idStr, ok := id.(string)
		s.Require().True(ok, "the _id of an insert op in the captured oplog is a string")
		ids = append(ids, idStr)
	})

	return ids
}

// lastTimestampInOplogFile returns the newest timestamp in a dumped oplog.bson.
// That is the dump's point in time: mongodump captured an end timestamp and wrote
// out every entry up to it, so no later entry exists that it left out.
func (s *DumpRestoreSuite) lastTimestampInOplogFile(path string) bson.Timestamp {
	var last bson.Timestamp
	s.eachOpInFile(path, func(op db.Oplog) {
		if util.TimestampGreaterThan(op.Timestamp, last) {
			last = op.Timestamp
		}
	})
	s.Require().NotZero(last, "the captured oplog is not empty")

	return last
}

func (s *DumpRestoreSuite) eachOpInFile(path string, handle func(db.Oplog)) {
	file, err := os.Open(path)
	s.Require().NoError(err, "the dump contains a captured oplog at %#q", path)
	defer file.Close()

	source := db.NewDecodedBSONSource(db.NewBSONSource(file))
	defer source.Close()

	op := db.Oplog{}
	for source.Next(&op) {
		handle(op)
	}
	s.Require().NoError(source.Err(), "can read the captured oplog")
}

// insertedIDsFromServerOplog returns the _ids inserted into ns after since and at
// or before upTo, read from the server's own oplog. That is the set of documents a
// point-in-time snapshot taken at upTo has to contain.
func (s *DumpRestoreSuite) insertedIDsFromServerOplog(
	ns string,
	since, upTo bson.Timestamp,
) []string {
	session, err := testutil.GetBareSession(s.T())
	s.Require().NoError(err, "can connect to the server")

	cursor, err := session.Database("local").Collection("oplog.rs").Find(
		s.Context(),
		bson.D{
			{"ns", ns},
			{"op", "i"},
			{"ts", bson.D{{"$gt", since}, {"$lte", upTo}}},
		},
	)
	s.Require().NoError(err, "can read the server's oplog")

	var ops []struct {
		Object stringIDDoc `bson:"o"`
	}
	s.Require().NoError(cursor.All(s.Context(), &ops), "can decode the server's oplog entries")

	return lo.Map(ops, func(op struct {
		Object stringIDDoc `bson:"o"`
	}, _ int) string {
		return op.Object.ID
	})
}
