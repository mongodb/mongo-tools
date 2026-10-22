//go:build integration

package dumprestore

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// timeseriesSeedCount is the number of measurements seeded before the oplog checkpoint in the
// timeseries oplog-replay tests.
const timeseriesSeedCount = 100

// tsMeasurement is the projection of a timeseries measurement compared between source and target.
// `_id` is left out because the server assigns it. We store our own id value in the "id" field to
// make it easy to sort these docs after we retrieve them. The `a` and `e` fields just hold some
// arbitrary values so that these docs have some additional data.
type tsMeasurement struct {
	TS   bson.DateTime `bson:"ts"`
	Meta string        `bson:"m"`
	ID   int32         `bson:"id"`
	A    int32         `bson:"a"`
	E    int32         `bson:"e"`
}

func (tm tsMeasurement) toBSONDoc() bson.D {
	bytes, err := bson.Marshal(tm)
	if err != nil {
		panic(err)
	}

	var doc bson.D
	err = bson.Unmarshal(bytes, &doc)
	if err != nil {
		panic(err)
	}

	return doc
}

// TestOplogReplayTimeseriesBucketDiffsBeforeWindow reproduces TOOLS-4373: replaying a `mongodump
// --oplog` window onto a timeseries bucket can lead to a bucket-validation error (Location7667503,
// "Invalid BSON Column encoding").
//
// `mongodump --oplog` records the oplog start and only then scans the collections. The failure
// happens when a timeseries bucket is created before the oplog start and that bucket contains some
// writes also present in the oplog. In other words, if the data dumped from the collection overlaps
// with the oplog, that can trigger the error.
//
// This test builds that same overlap deterministically: it checkpoints the oplog, mutates a
// pre-existing bucket, dumps the bucket's final state, then replays only the window onto that
// dump. Each case is a different mutation, because the fix relies on later ops repairing the
// intermediate state. We want to test that this works for deletes, updates, out-of-order inserts,
// and combinations of those, not just in-order appends.
func (s *DumpRestoreSuite) TestOplogReplayTimeseriesBucketDiffsBeforeWindow() {
	testutil.SkipUnlessReplicaSet(s.T())
	testutil.SkipForDisaggregatedStorage(
		s.T(),
		"it replays an oplog, and DSC does not support the applyOps command",
	)
	// The compressed bucket diffs this exercises were introduced in 8.0.
	testutil.SkipIfFCVLessThan(s.T(), "8.0", "compressed bucket diffs require FCV 8.0")

	// We anchor the measurements inside one bucket time range (a single hour for a collection with
	// seconds granularity) so the cases work against one bucket unless they deliberately span more.
	base := time.Now().Truncate(time.Hour)
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Second) }

	const (
		metaVal1 = "m1"
		metaVal2 = "m2"
	)
	oneMeta := func(int) string { return metaVal1 }
	alternatingMeta := func(i int) string { return fmt.Sprintf("m%d", i%2) }

	testCases := []struct {
		name   string
		meta   func(int) string
		mutate func(*mongo.Collection)
	}{
		{
			name: "in-order appends",
			meta: oneMeta,
			mutate: func(coll *mongo.Collection) {
				s.insertMeasurements(coll, rangeDocs(timeseriesSeedCount, 800, at, oneMeta))
			},
		},
		{
			name: "out-of-order inserts",
			meta: oneMeta,
			mutate: func(coll *mongo.Collection) {
				// Half-second timestamps land between every seeded measurement, so the bucket is
				// reopened and re-compressed rather than appended to at the end.
				odd := func(i int) time.Time {
					return base.Add(
						time.Duration(i-timeseriesSeedCount)*time.Second + 500*time.Millisecond,
					)
				}
				s.insertMeasurements(
					coll,
					rangeDocs(timeseriesSeedCount, timeseriesSeedCount+100, odd, oneMeta),
				)
			},
		},
		{
			name: "deletes",
			meta: oneMeta,
			mutate: func(coll *mongo.Collection) {
				s.deleteEvenMeasurements(coll)
			},
		},
		{
			name: "metaField update",
			meta: oneMeta,
			mutate: func(coll *mongo.Collection) {
				// Time-series updates may only filter and set the metaField.
				_, err := coll.UpdateMany(
					s.Context(),
					bson.D{{"m", metaVal1}},
					bson.D{{"$set", bson.D{{"m", metaVal2}}}},
				)
				s.Require().NoError(err, "can update the metaField")
			},
		},
		{
			name: "delete and reinsert",
			meta: oneMeta,
			mutate: func(coll *mongo.Collection) {
				s.deleteEvenMeasurements(coll)
				s.insertMeasurements(
					coll,
					rangeDocs(timeseriesSeedCount, timeseriesSeedCount+50, at, oneMeta),
				)
			},
		},
		{
			name: "multiple meta values",
			meta: alternatingMeta,
			mutate: func(coll *mongo.Collection) {
				s.insertMeasurements(coll, rangeDocs(timeseriesSeedCount, 800, at, alternatingMeta))
			},
		},
		{
			name: "mixed",
			meta: oneMeta,
			mutate: func(coll *mongo.Collection) {
				s.insertMeasurements(coll, rangeDocs(timeseriesSeedCount, 300, at, oneMeta))
				s.deleteEvenMeasurements(coll)
				_, err := coll.UpdateMany(
					s.Context(),
					bson.D{{"m", metaVal1}},
					bson.D{{"$set", bson.D{{"m", metaVal2}}}},
				)
				s.Require().NoError(err, "can update the metaField")
			},
		},
	}

	for _, tc := range testCases {
		s.Run(
			tc.name,
			func() {
				s.assertOplogReplayTimeseriesBucketDiffs(at, tc.meta, tc.mutate)
			},
		)
	}
}

// assertOplogReplayTimeseriesBucketDiffs runs one mutation through the dump/capture/replay cycle.
// It seeds measurements for the given metaField distribution, checkpoints the oplog, applies
// mutate, captures the resulting diffs, and replays them onto the dump's final state. It asserts
// the restore succeeds, the measurements match the source, and the bucket is valid.
func (s *DumpRestoreSuite) assertOplogReplayTimeseriesBucketDiffs(
	at func(int) time.Time,
	meta func(int) string,
	mutate func(*mongo.Collection),
) {
	ctx := s.Context()
	const collName = "metrics"

	testDB := s.database("oplog_ts_bucket")
	dbName := testDB.Name()
	bucketColl := s.bucketCollectionName(collName)
	bucketNS := dbName + "." + bucketColl

	s.Require().NoError(testDB.Drop(ctx), "start from a clean database")

	s.createTimeseriesCollection(testDB, collName)

	coll := testDB.Collection(collName)
	s.insertMeasurements(coll, rangeDocs(0, timeseriesSeedCount, at, meta))

	// Everything the mutation writes after this checkpoint is replayed as a compressed-column
	// diff, with no bucket insert in the window to reset the pre-existing bucket first.
	checkpoint := s.latestOplogTimestamp()

	mutate(coll)

	// Snapshot the expected measurements before the source is dropped.
	want := s.readMeasurements(coll)
	s.Require().NotEmpty(want, "the mutation left measurements to restore")

	// Capture just the mutation's diffs, the way mongodump --oplog would have captured them. We
	// can't do this by simply running `mongodump --oplog` because we need to make sure that the
	// oplog dump contains just the ops created by the `mutate` func.
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

func (s *DumpRestoreSuite) deleteEvenMeasurements(coll *mongo.Collection) {
	_, err := coll.DeleteMany(s.Context(), bson.D{{"id", bson.D{{"$mod", bson.A{2, 0}}}}})
	s.Require().NoError(err, "can delete every other measurement")
}

// TestOplogReplayTimeseriesBucketCreatedInWindow covers a bucket that does not exist when the data
// dump (not oplog dump) starts and is created while the oplog is being dumped. `mongodump --oplog`
// captures the bucket's creation and replay reconstructs the bucket.
//
// The bucket creation is not a top-level insert: when a single write batch creates several buckets
// the server groups their inserts into one `applyOps` command logged on `admin.$cmd`. An oplog
// query that filters on the bucket namespace would drop that command, which is why this test
// captures the whole window, the way mongodump does, and asserts a grouped insert is present.
func (s *DumpRestoreSuite) TestOplogReplayTimeseriesBucketCreatedInWindow() {
	testutil.SkipUnlessReplicaSet(s.T())
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

	s.createTimeseriesCollection(testDB, collName)

	coll := testDB.Collection(collName)
	s.insertMeasurements(coll, rangeDocs(0, 100, at, meta))

	// The oplog window opens here, before the archive is read.
	checkpoint := s.latestOplogTimestamp()

	// These buckets exist when the archive is read below.
	s.insertMeasurements(coll, rangeDocs(100, 4000, at, meta))

	s.withBSONMongodump(func(dir string) {
		// These writes come after the archive's read point, so the buckets they create are not in
		// the archive. The window below captures their creation ops.
		s.insertMeasurements(coll, rangeDocs(4000, 8000, at, meta))

		want := s.readMeasurements(coll)

		// Capture the whole window, the way mongodump --oplog does. Filtering on the bucket
		// namespace would drop the applyOps commands for multi-bucket inserts.
		oplogDir, cleanupOplog := testutil.MakeTempDir(s.T())
		defer cleanupOplog()
		s.runMongodumpWithArgs(
			"--out", oplogDir,
			"--db", "local",
			"--collection", "oplog.rs",
			"--query", fmt.Sprintf(
				`{"ts": {"$gt": {"$timestamp": {"t": %d, "i": %d}}}}`,
				checkpoint.T,
				checkpoint.I,
			),
		)
		oplogPath := filepath.Join(oplogDir, "local", "oplog.rs.bson")

		// Confirm the scenario actually occurs: the window must create buckets the archive does not
		// contain, grouped into an applyOps command. Otherwise this test proves nothing.
		s.Require().True(
			s.windowHasGroupedBucketInserts(oplogPath, bucketNS),
			"the window creates buckets in an applyOps command",
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

func (s *DumpRestoreSuite) createTimeseriesCollection(testDB *mongo.Database, collName string) {
	tsType := reflect.TypeFor[tsMeasurement]()

	f, exists := tsType.FieldByName("TS")
	s.Require().True(exists, "tsMeasurement struct has TS field")
	tsKeyName := f.Tag.Get("bson")

	f, exists = tsType.FieldByName("Meta")
	s.Require().True(exists, "tsMeasurement struct has Meta field")
	metaKeyName := f.Tag.Get("bson")

	createOpts := mopt.CreateCollection().SetTimeSeriesOptions(
		mopt.TimeSeries().
			SetTimeField(tsKeyName).
			SetMetaField(metaKeyName).
			SetGranularity("seconds"),
	)
	s.Require().NoError(
		testDB.CreateCollection(s.Context(), collName, createOpts),
		"can create the timeseries collection",
	)
}

// rangeDocs builds measurements for the ids in [from, to), with timestamps from ts and meta values
// from meta. The ids are stored in the `id` field so the tests can compare and sort by them.
func rangeDocs(from, to int, ts func(int) time.Time, meta func(int) string) []bson.D {
	docs := make([]bson.D, 0, to-from)
	for i := from; i < to; i++ {
		doc := tsMeasurement{
			TS:   bson.NewDateTimeFromTime(ts(i)),
			Meta: meta(i),
			ID:   int32(i),
			A:    int32(i % 97),
			E:    int32(i % 7),
		}.toBSONDoc()
		docs = append(docs, doc)
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

// windowHasGroupedBucketInserts reports whether the captured oplog window contains an applyOps
// command that inserts into the buckets namespace. The server groups the inserts of a batch that
// creates several buckets into one such command, logged on admin.$cmd rather than the buckets
// namespace, so a query filtered on the buckets namespace would not capture it.
func (s *DumpRestoreSuite) windowHasGroupedBucketInserts(path, bucketNS string) bool {
	file, err := os.Open(path)
	s.Require().NoError(err, "the captured oplog exists at %#q", path)
	defer file.Close()

	source := db.NewDecodedBSONSource(db.NewBSONSource(file))
	defer source.Close()

	op := db.Oplog{}
	for source.Next(&op) {
		if op.Operation != "c" {
			continue
		}

		for _, e := range op.Object {
			if e.Key != "applyOps" {
				continue
			}

			inner, ok := e.Value.(bson.A)
			if !ok {
				continue
			}

			for _, item := range inner {
				raw, err := bson.Marshal(item)
				if err != nil {
					continue
				}

				var nested db.Oplog
				if bson.Unmarshal(raw, &nested) != nil {
					continue
				}

				if nested.Operation == "i" && nested.Namespace == bucketNS {
					return true
				}
			}
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

	sort.Slice(measurements, func(i, j int) bool { return measurements[i].ID < measurements[j].ID })

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
