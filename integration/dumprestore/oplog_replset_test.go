//go:build integration

package dumprestore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestOplogReplayFromLocalOplogRS dumps local.oplog.rs with a $gt timestamp
// query, capturing only the ops written after a checkpoint, then replays that
// dump with --oplogReplay --oplogFile and checks exactly those ops are applied
// and the ops preceding the checkpoint are not.
func (s *DumpRestoreSuite) TestOplogReplayFromLocalOplogRS() {
	// local.oplog.rs only exists on a replica set. The suite as a whole gates on
	// the integration test type, so this case needs a gate of its own.
	testutil.SkipUnlessReplicaSet(s.T())
	testutil.SkipForDisaggregatedStorage(
		s.T(),
		"it replays an oplog, and DSC does not support the applyOps command",
	)

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

// TestDumpOplogSnapshotDuringWrites checks the point-in-time guarantee `--oplog` makes about a dump
// taken while writes are in flight: the collection data plus the captured oplog together account
// for every document that existed when the dump finished. A dump without `--oplog` cannot promise
// that, because documents inserted after its cursor passed them are in neither half.
//
// The guarantee is checked against the two halves directly rather than by restoring with
// `--oplogReplay`. mongorestore rejects `--oplogReplay` alongside `--nsInclude`, and `--oplog` is
// only accepted on a full dump, so a replaying restore would have to write the whole cluster back
// - including admin and config - to test one collection. What replay would produce is exactly the
// union asserted here, and TestOplogReplayFromLocalOplogRS covers the replay machinery itself.
func (s *DumpRestoreSuite) TestDumpOplogSnapshotDuringWrites() {
	// The oplog only exists on a replica set. The suite as a whole gates on the
	// integration test type, so this case needs a gate of its own.
	testutil.SkipUnlessReplicaSet(s.T())

	const collName = "bar"

	testDB := s.database("oplog_snapshot")
	coll := testDB.Collection(collName)

	// Dropping the database does not clear the server's oplog, so earlier runs of this test could
	// have written entries for this same namespace that are still there. The lookup of the
	// concurrent inserts below starts from here to leave them out.
	since := s.latestOplogTimestamp()

	s.insertNamespacedDocs(coll)
	preexistingIDs := s.documentIDs(coll)
	countBefore := s.docCount(coll)
	s.Require().Positive(countBefore, "the collection holds documents before the dump")

	ns := testDB.Name() + "." + collName

	var wantIDs, oplogIDs []string
	s.withConcurrentInserts(
		coll,
		func(stopInserts func()) {
			s.withBSONMongodump(func(dir string) {
				stopInserts()

				s.Require().Greater(
					s.docCount(coll),
					countBefore,
					"the concurrent inserts landed while the dump was running",
				)

				oplogPath := filepath.Join(dir, "oplog.bson")
				var lastTS bson.Timestamp
				oplogIDs, lastTS = s.idsAndLastTimestampFromOplogFile(oplogPath, ns)

				// The dump's point in time is where its captured oplog ends, which is earlier than now:
				// mongodump takes that timestamp and then still has to write the oplog out and exit,
				// and the inserts ran until it did.
				//
				// The pre-existing documents are added separately because they predate the dump either
				// way, and because they went in as one batch, which the server records as a single
				// applyOps entry rather than an insert op each.
				wantIDs = lo.Union(preexistingIDs, s.insertedIDsFromServerOplog(ns, since, lastTS))

				s.dropDB(testDB)

				result := s.runRestore(mongorestore.NSIncludeOption, testDB.Name()+".*", dir)
				s.Require().NoError(result.Err, "can restore the collection half of the dump")
			}, "--oplog")
		},
	)

	// Without this the union below could hold whatever the data half happened to
	// hold, and would still match if --oplog had captured nothing at all.
	s.Require().NotEmpty(
		oplogIDs,
		"the captured oplog holds inserts, so the dump really did span concurrent writes",
	)

	s.Assert().ElementsMatch(
		wantIDs,
		lo.Union(s.documentIDs(coll), oplogIDs),
		"the dumped data and the captured oplog together account for exactly the documents that existed at the dump's point in time",
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

// idsAndLastTimestampFromOplogFile reads an oplog file. It returns all the `_id` values in the
// oplog, as well as the timestamp of the last op.
func (s *DumpRestoreSuite) idsAndLastTimestampFromOplogFile(
	path, ns string,
) ([]string, bson.Timestamp) {
	var (
		ids    []string
		lastTS bson.Timestamp
	)

	file, err := os.Open(path)
	s.Require().NoError(err, "the dump contains a captured oplog at %#q", path)
	defer file.Close()

	source := db.NewDecodedBSONSource(db.NewBSONSource(file))
	defer source.Close()

	op := db.Oplog{}
	for source.Next(&op) {
		if util.TimestampGreaterThan(op.Timestamp, lastTS) {
			lastTS = op.Timestamp
		}

		if op.Operation != "i" || op.Namespace != ns {
			continue
		}

		id, err := bsonutil.FindValueByKey("_id", &op.Object)
		s.Require().NoError(err, "an insert op in the captured oplog has an _id")

		idStr, ok := id.(string)
		s.Require().True(ok, "the _id of an insert op in the captured oplog is a string")
		ids = append(ids, idStr)

	}
	s.Require().NoError(source.Err(), "can read the captured oplog")

	return ids, lastTS
}

// insertedIDsFromServerOplog returns the _ids inserted into ns after since and at or before upTo,
// read from the server's own oplog. That is the set of documents a point-in-time snapshot taken at
// upTo has to contain.
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

// TestOplogReplayTimeseriesBucketDiffsBeforeWindow reproduces TOOLS-4373: replaying a `mongodump
// --oplog` window onto a timeseries bucket that predates the window aborts the whole restore with a
// bucket-validation error (Location7667503, "Invalid BSON Column encoding").
//
// The failure does not need a live writer to trigger. `mongodump --oplog` records the oplog start
// and only then scans the collections, so a bucket created before that start can be captured in the
// dump after it has already absorbed some of the writes that the captured window also holds.
// Replaying those writes truncates the compressed column at an offset computed from the earlier
// state, which lands inside a repacked Simple8b block and leaves a malformed column for the per-op
// validation in applyOps to reject. This test builds that same overlap deterministically: it
// checkpoints the oplog, mutates a pre-existing bucket, dumps the bucket's final state, then replays
// only the window onto that dump. Each case is a different mutation, because the fix relies on later
// diffs repairing the intermediate state and that has to hold for deletes, updates, out-of-order
// inserts, and combinations, not just in-order appends.
func (s *DumpRestoreSuite) TestOplogReplayTimeseriesBucketDiffsBeforeWindow() {
	testutil.SkipForDisaggregatedStorage(
		s.T(),
		"it replays an oplog, and DSC does not support the applyOps command",
	)
	// The compressed bucket diffs this exercises are written by 8.0 and later.
	testutil.SkipIfFCVLessThan(s.T(), "8.0", "compressed bucket diffs require FCV 8.0")

	const seedCount = 100

	// Anchor the measurements inside one bucket time range (a single hour for seconds
	// granularity) so the cases work against one bucket unless they deliberately span more.
	base := time.Now().Truncate(time.Hour)
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Second) }

	oneMeta := func(int) string { return "same" }
	alternatingMeta := func(i int) string { return fmt.Sprintf("m%d", i%2) }

	seed := func(meta func(int) string) func(*mongo.Collection) {
		return func(coll *mongo.Collection) {
			s.insertMeasurements(coll, rangeDocs(0, seedCount, at, meta))
		}
	}

	testCases := []struct {
		name   string
		seed   func(*mongo.Collection)
		mutate func(*mongo.Collection)
	}{
		{
			name: "in-order appends",
			seed: seed(oneMeta),
			mutate: func(coll *mongo.Collection) {
				s.insertMeasurements(coll, rangeDocs(seedCount, 800, at, oneMeta))
			},
		},
		{
			name: "out-of-order inserts",
			seed: seed(oneMeta),
			mutate: func(coll *mongo.Collection) {
				// Half-second timestamps land between every seeded measurement, so the bucket
				// is reopened and re-compressed rather than appended to at the end.
				odd := func(i int) time.Time {
					return base.Add(time.Duration(i-seedCount)*time.Second + 500*time.Millisecond)
				}
				s.insertMeasurements(
					coll,
					rangeDocs(seedCount, seedCount+100, odd, oneMeta),
				)
			},
		},
		{
			name: "deletes",
			seed: seed(oneMeta),
			mutate: func(coll *mongo.Collection) {
				s.deleteEvenMeasurements(coll)
			},
		},
		{
			name: "metaField update",
			seed: seed(oneMeta),
			mutate: func(coll *mongo.Collection) {
				// Time-series updates may only filter and set the metaField.
				_, err := coll.UpdateMany(
					s.Context(),
					bson.D{{"m", "same"}},
					bson.D{{"$set", bson.D{{"m", "other"}}}},
				)
				s.Require().NoError(err, "can update the metaField")
			},
		},
		{
			name: "delete and reinsert",
			seed: seed(oneMeta),
			mutate: func(coll *mongo.Collection) {
				s.deleteEvenMeasurements(coll)
				s.insertMeasurements(coll, rangeDocs(seedCount, seedCount+50, at, oneMeta))
			},
		},
		{
			name: "multiple meta values",
			seed: seed(alternatingMeta),
			mutate: func(coll *mongo.Collection) {
				s.insertMeasurements(coll, rangeDocs(seedCount, 800, at, alternatingMeta))
			},
		},
		{
			name: "mixed",
			seed: seed(oneMeta),
			mutate: func(coll *mongo.Collection) {
				s.insertMeasurements(coll, rangeDocs(seedCount, 300, at, oneMeta))
				s.deleteEvenMeasurements(coll)
				_, err := coll.UpdateMany(
					s.Context(),
					bson.D{{"m", "same"}},
					bson.D{{"$set", bson.D{{"m", "other"}}}},
				)
				s.Require().NoError(err, "can update the metaField")
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.assertOplogReplayTimeseriesBucketDiffs(tc.seed, tc.mutate)
		})
	}
}

// TestOplogReplayTimeseriesBucketCreatedInWindow exercises a replay failure that is distinct from
// TOOLS-4373 and that the per-op validation bypass does not address. It does not depend on the
// bypass at all: it fails the same way with and without it.
//
// The failure is a bucket that does not exist at the archive's read point and is created inside the
// oplog window by a compressed-column diff (op "u", $v:2) rather than an insert (op "i"). applyOps
// cannot materialize a bucket from a diff, so replay aborts with Location6781400 ("Time series
// bucket document is missing 'control' field") before any final validation could run.
//
// The read point is placed after the first half of the writes, so the second half creates buckets
// for a new hour under meta values that already exist. That write shape makes the server record
// bucket creation as a diff instead of an insert, which is what the test relies on; it asserts the
// precondition holds so a server that starts emitting inserts cannot make the test pass vacuously.
func (s *DumpRestoreSuite) TestOplogReplayTimeseriesBucketCreatedInWindow() {
	testutil.SkipForDisaggregatedStorage(
		s.T(),
		"it replays an oplog, and DSC does not support the applyOps command",
	)
	testutil.SkipIfFCVLessThan(s.T(), "8.0", "compressed bucket diffs require FCV 8.0")

	const collName = "metrics"

	base := time.Now().Truncate(time.Hour)
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Second) }
	// Cycling a small set of meta values means the second half of the writes opens a new hour for
	// meta values that already have a bucket, which is the shape that produces diff-style creation.
	meta := func(i int) string { return fmt.Sprintf("m%d", i%50) }

	testDB := s.database("oplog_ts_bucket_in_window")
	dbName := testDB.Name()
	bucketColl := s.bucketCollectionName(collName)
	bucketNS := dbName + "." + bucketColl

	s.Require().NoError(testDB.Drop(s.Context()), "start from a clean database")

	createOpts := mopt.CreateCollection().SetTimeSeriesOptions(
		mopt.TimeSeries().SetTimeField("ts").SetMetaField("m").SetGranularity("seconds"),
	)
	s.Require().NoError(
		testDB.CreateCollection(s.Context(), collName, createOpts),
		"can create the timeseries collection",
	)

	coll := testDB.Collection(collName)
	s.insertMeasurements(coll, rangeDocs(0, 100, at, meta))

	// The oplog window opens here, before the archive is read.
	checkpoint := s.latestOplogTimestamp()

	// These buckets exist when the archive is read below.
	s.insertMeasurements(coll, rangeDocs(100, 4000, at, meta))

	s.withBSONMongodump(func(dir string) {
		// These writes land after the archive's read point, so the buckets they create are not in
		// the archive. The window below captures their creation ops.
		s.insertMeasurements(coll, rangeDocs(4000, 8000, at, meta))

		want := s.readMeasurements(coll)

		oplogDir, cleanupOplog := testutil.MakeTempDir(s.T())
		defer cleanupOplog()
		s.runMongodumpWithArgs(
			"--out", oplogDir,
			"--db", "local",
			"--collection", "oplog.rs",
			"--query", fmt.Sprintf(
				`{"ts": {"$gt": {"$timestamp": {"t": %d, "i": %d}}}, "ns": %q}`,
				checkpoint.T,
				checkpoint.I,
				bucketNS,
			),
		)
		oplogPath := filepath.Join(oplogDir, "local", "oplog.rs.bson")

		// Confirm the scenario actually occurs: the window must create, by diff, a bucket the
		// archive does not hold. Otherwise this test proves nothing.
		archiveIDs := s.archiveBucketIDs(filepath.Join(dir, dbName, bucketColl+".bson"))
		s.Require().True(
			s.windowCreatesMissingBucketByDiff(oplogPath, archiveIDs),
			"the window creates a bucket by diff that the archive lacks",
		)

		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.OplogReplayOption,
			mongorestore.OplogFileOption, oplogPath,
			mongorestore.DirectoryOption, dir,
		)
		s.Require().NoError(
			result.Err,
			"replaying a window that creates buckets the archive lacks succeeds",
		)

		s.Assert().Equal(
			want,
			s.readMeasurements(coll),
			"the restored measurements match the source",
		)

		s.assertBucketIsValid(testDB, bucketColl)
	}, "--db", dbName)
}

// archiveBucketIDs returns the _id of every bucket document in a dumped bucket collection.
func (s *DumpRestoreSuite) archiveBucketIDs(path string) map[any]bool {
	file, err := os.Open(path)
	s.Require().NoError(err, "the dumped bucket collection exists at %#q", path)
	defer file.Close()

	source := db.NewDecodedBSONSource(db.NewBSONSource(file))
	defer source.Close()

	ids := map[any]bool{}
	var doc struct {
		ID any `bson:"_id"`
	}
	for source.Next(&doc) {
		ids[doc.ID] = true
	}
	s.Require().NoError(source.Err(), "can read the dumped bucket collection")

	return ids
}

// windowCreatesMissingBucketByDiff reports whether the captured oplog window contains a compressed
// diff update whose target bucket is absent from archiveIDs.
func (s *DumpRestoreSuite) windowCreatesMissingBucketByDiff(
	path string,
	archiveIDs map[any]bool,
) bool {
	file, err := os.Open(path)
	s.Require().NoError(err, "the captured oplog exists at %#q", path)
	defer file.Close()

	source := db.NewDecodedBSONSource(db.NewBSONSource(file))
	defer source.Close()

	op := db.Oplog{}
	for source.Next(&op) {
		if op.Operation != "u" {
			continue
		}

		var id any
		for _, e := range op.Query {
			if e.Key == "_id" {
				id = e.Value
			}
		}

		if id != nil && !archiveIDs[id] {
			return true
		}
	}
	s.Require().NoError(source.Err(), "can read the captured oplog")

	return false
}

// bucketCollectionName returns the namespace a timeseries collection's buckets are written to, which
// is where both the oplog records bucket writes and `validate` finds the buckets. Bucket writes use
// the internal system.buckets.<coll> namespace up to FCV 9.0; viewless collections (FCV 9.0+) write
// them under the user-facing collection name. This is a later boundary than SupportsRawData (8.3),
// which only controls the rawData read API.
func (s *DumpRestoreSuite) bucketCollectionName(collName string) string {
	fcv := testutil.GetFCV(s.Client())
	if cmp, err := testutil.CompareFCV(fcv, "9.0"); err == nil && cmp >= 0 {
		return collName
	}

	return common.TimeseriesBucketPrefix + collName
}

// assertOplogReplayTimeseriesBucketDiffs runs one mutation through the dump/capture/replay cycle.
// seed creates the bucket(s), the oplog is checkpointed, mutate changes the collection, the
// resulting diffs are captured, and the dump's final state is replayed with only those diffs. It
// asserts the restore succeeds, the measurements match the source, and the bucket is valid.
func (s *DumpRestoreSuite) assertOplogReplayTimeseriesBucketDiffs(
	seed, mutate func(*mongo.Collection),
) {
	ctx := s.Context()
	const collName = "metrics"

	testDB := s.database("oplog_ts_bucket")
	dbName := testDB.Name()
	bucketColl := s.bucketCollectionName(collName)
	bucketNS := dbName + "." + bucketColl

	s.Require().NoError(testDB.Drop(ctx), "start from a clean database")

	createOpts := mopt.CreateCollection().SetTimeSeriesOptions(
		mopt.TimeSeries().SetTimeField("ts").SetMetaField("m").SetGranularity("seconds"),
	)
	s.Require().NoError(
		testDB.CreateCollection(ctx, collName, createOpts),
		"can create the timeseries collection",
	)

	coll := testDB.Collection(collName)
	seed(coll)

	// Everything the mutation writes after this checkpoint is replayed as a compressed-column
	// diff, with no bucket insert in the window to reset the pre-existing bucket first.
	checkpoint := s.latestOplogTimestamp()

	mutate(coll)

	// Snapshot the expected measurements before the source is dropped.
	want := s.readMeasurements(coll)
	s.Require().NotEmpty(want, "the mutation left measurements to restore")

	// Capture just the mutation's diffs, the way mongodump --oplog would have captured them.
	oplogDir, cleanupOplog := testutil.MakeTempDir(s.T())
	defer cleanupOplog()
	s.runMongodumpWithArgs(
		"--out", oplogDir,
		"--db", "local",
		"--collection", "oplog.rs",
		"--query", fmt.Sprintf(
			`{"ts": {"$gt": {"$timestamp": {"t": %d, "i": %d}}}, "ns": %q}`,
			checkpoint.T,
			checkpoint.I,
			bucketNS,
		),
	)

	// Without this the restore below would pass on an empty window, proving nothing.
	oplogPath := filepath.Join(oplogDir, "local", "oplog.rs.bson")
	s.Require().Positive(
		s.capturedOplogOpCount(oplogPath),
		"the captured oplog window holds ops to replay",
	)

	// Dump the bucket's final state, drop it, and replay the diffs onto that state. The dump is
	// ahead of the first diff, which is what `mongodump --oplog` produces for a bucket that
	// existed before its oplog window.
	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(
			mongorestore.OplogReplayOption,
			mongorestore.OplogFileOption, oplogPath,
			mongorestore.DirectoryOption, dir,
		)
		s.Require().NoError(
			result.Err,
			"replaying bucket diffs onto a dump that already contains them succeeds",
		)

		s.Assert().Equal(
			want,
			s.readMeasurements(coll),
			"the restored measurements match the source",
		)

		s.assertBucketIsValid(testDB, bucketColl)
	}, "--db", dbName)
}

// bucketCollectionName returns the namespace a timeseries collection's buckets are written to, which
// is where both the oplog records bucket writes and `validate` finds the buckets. Bucket writes use
// the internal system.buckets.<coll> namespace up to FCV 9.0; viewless collections (FCV 9.0+) write
// them under the user-facing collection name. This is a later boundary than SupportsRawData (8.3),
// which only controls the rawData read API.
func (s *DumpRestoreSuite) bucketCollectionName(collName string) string {
	fcv := testutil.GetFCV(s.Client())
	if cmp, err := testutil.CompareFCV(fcv, "9.0"); err == nil && cmp >= 0 {
		return collName
	}

	return common.TimeseriesBucketPrefix + collName
}

// tsMeasurement is the projection of a timeseries measurement compared between source and target.
// `_id` is left out because the server assigns it.
type tsMeasurement struct {
	TS bson.DateTime `bson:"ts"`
	M  string        `bson:"m"`
	A  int32         `bson:"a"`
	C  int32         `bson:"c"`
	E  int32         `bson:"e"`
}

// readMeasurements reads a timeseries collection's measurements in a stable order, so source and
// target can be compared directly.
func (s *DumpRestoreSuite) readMeasurements(coll *mongo.Collection) []tsMeasurement {
	cursor, err := coll.Find(
		s.Context(),
		bson.D{},
		mopt.Find().SetProjection(bson.D{{"_id", 0}}),
	)
	s.Require().NoError(err, "can read measurements from %#q", coll.Name())

	var measurements []tsMeasurement
	s.Require().
		NoError(cursor.All(s.Context(), &measurements), "can decode measurements from %#q", coll.Name())

	sort.Slice(measurements, func(i, j int) bool { return measurements[i].C < measurements[j].C })

	return measurements
}

// capturedOplogOpCount returns the number of entries in a captured oplog file.
func (s *DumpRestoreSuite) capturedOplogOpCount(path string) int {
	file, err := os.Open(path)
	s.Require().NoError(err, "the captured oplog exists at %#q", path)
	defer file.Close()

	source := db.NewDecodedBSONSource(db.NewBSONSource(file))
	defer source.Close()

	var total int
	op := db.Oplog{}
	for source.Next(&op) {
		total++
	}
	s.Require().NoError(source.Err(), "can read the captured oplog")

	return total
}

// assertBucketIsValid runs the full BSON-conformance validation on the restored bucket collection.
// It catches a replay that converged on the right measurement count but left a malformed column.
func (s *DumpRestoreSuite) assertBucketIsValid(testDB *mongo.Database, bucketColl string) {
	var result bson.M
	err := testDB.RunCommand(s.Context(), bson.D{
		{"validate", bucketColl},
		{"full", true},
		{"checkBSONConformance", true},
	}).Decode(&result)
	s.Require().NoError(err, "can validate the restored bucket collection %#q", bucketColl)
	s.Assert().EqualValues(
		true,
		result["valid"],
		"the restored bucket is valid: %v",
		result["errors"],
	)
}

func (s *DumpRestoreSuite) deleteEvenMeasurements(coll *mongo.Collection) {
	_, err := coll.DeleteMany(s.Context(), bson.D{{"c", bson.D{{"$mod", bson.A{2, 0}}}}})
	s.Require().NoError(err, "can delete every other measurement")
}

// rangeDocs builds measurements for the ids in [from, to), with timestamps from ts and meta values
// from meta. The ids are stored in the `c` field so the tests can compare and sort by them.
func rangeDocs(from, to int, ts func(int) time.Time, meta func(int) string) []bson.D {
	docs := make([]bson.D, 0, to-from)
	for i := from; i < to; i++ {
		docs = append(docs, bson.D{
			{"ts", bson.NewDateTimeFromTime(ts(i))},
			{"m", meta(i)},
			{"a", int32(i % 97)},
			{"c", int32(i)},
			{"e", int32(i % 7)},
		})
	}

	return docs
}

// measurementBatchSize is deliberately small: each batch is recorded as its own compressed-column
// diff in the oplog, so a bucket accumulates many of the diffs the replay has to apply.
const measurementBatchSize = 5

// insertMeasurements inserts measurements in small batches.
func (s *DumpRestoreSuite) insertMeasurements(coll *mongo.Collection, docs []bson.D) {
	ctx := s.Context()
	for start := 0; start < len(docs); start += measurementBatchSize {
		end := min(start+measurementBatchSize, len(docs))

		batch := make([]any, 0, end-start)
		for _, doc := range docs[start:end] {
			batch = append(batch, doc)
		}

		_, err := coll.InsertMany(ctx, batch)
		s.Require().NoError(err, "can insert measurements [%d, %d)", start, end)
	}
}
