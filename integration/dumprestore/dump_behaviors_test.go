package dumprestore

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type extendedJSONQueryCase struct {
	name string

	// matching and other are the two documents inserted. Only matching is
	// expected to survive the query.
	matching bson.D
	other    bson.D

	// query is passed to mongodump verbatim, as it would be on a command line.
	query string

	// wantXType, when set, is a zero value of the BSON type the matching
	// document's "x" field must come back as. Checking the type is the point:
	// an extended JSON value parsed as a plain subdocument would still match
	// nothing, but one parsed as the wrong BSON type could match by accident.
	wantXType any
}

// TestDumpExtendedJSONQuery checks that --query values written in extended JSON
// reach the server as the BSON types they denote, rather than as the documents
// they look like. Each case inserts one document the query should match and one
// it should not, so a query parsed into the wrong type matches neither and the
// count assertion fails.
func (s *DumpRestoreSuite) TestDumpExtendedJSONQuery() {
	// A fixed instant, so the query string and the stored value cannot disagree
	// about sub-millisecond precision.
	when := time.Date(2021, time.March, 4, 5, 6, 7, 0, time.UTC)
	otherWhen := when.Add(-24 * time.Hour)

	matchingID := bson.NewObjectID()
	otherID := bson.NewObjectID()

	cases := []extendedJSONQueryCase{
		{
			name:     "date",
			matching: bson.D{{"_id", 1}, {"x", when}},
			other:    bson.D{{"_id", 2}, {"x", otherWhen}},
			query: fmt.Sprintf(
				`{"x": {"$date": {"$numberLong": "%d"}}}`,
				when.UnixMilli(),
			),
			wantXType: bson.DateTime(0),
		},
		{
			name:      "regular expression",
			matching:  bson.D{{"_id", 1}, {"x", bson.Regex{Pattern: "bacon", Options: "i"}}},
			other:     bson.D{{"_id", 2}, {"x", bson.Regex{Pattern: "bacon"}}},
			query:     `{"x": {"$regularExpression": {"pattern": "bacon", "options": "i"}}}`,
			wantXType: bson.Regex{},
		},
		{
			name:     "object id",
			matching: bson.D{{"_id", matchingID}},
			other:    bson.D{{"_id", otherID}},
			query:    `{"_id": {"$oid": "` + matchingID.Hex() + `"}}`,
		},
		{
			name:      "min key",
			matching:  bson.D{{"_id", 1}, {"x", bson.MinKey{}}},
			other:     bson.D{{"_id", 2}, {"x", 1}},
			query:     `{"x": {"$minKey": 1}}`,
			wantXType: bson.MinKey{},
		},
		{
			name:      "max key",
			matching:  bson.D{{"_id", 1}, {"x", bson.MaxKey{}}},
			other:     bson.D{{"_id", 2}, {"x", 1}},
			query:     `{"x": {"$maxKey": 1}}`,
			wantXType: bson.MaxKey{},
		},
	}

	for _, queryCase := range cases {
		s.Run(queryCase.name, func() {
			s.assertExtendedJSONQueryFilters(queryCase)
		})
	}
}

func (s *DumpRestoreSuite) assertExtendedJSONQueryFilters(queryCase extendedJSONQueryCase) {
	const collName = "bar"

	// Spaces are legal in a subtest name but not in a database name.
	testDB := s.database("extjson_query_" + strings.ReplaceAll(queryCase.name, " ", "_"))
	coll := testDB.Collection(collName)

	_, err := coll.InsertMany(s.Context(), []any{queryCase.matching, queryCase.other})
	s.Require().NoError(err, "can insert the two documents")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore the filtered dump")
		s.Require().EqualValues(
			1,
			result.Successes,
			"the query selected exactly the one matching document",
		)
	},
		"--db", testDB.Name(),
		"--collection", collName,
		"--query", queryCase.query,
	)

	var restored []struct {
		ID any `bson:"_id"`
		X  any `bson:"x"`
	}
	cursor, err := coll.Find(s.Context(), bson.D{})
	s.Require().NoError(err, "can read the restored documents")
	s.Require().NoError(cursor.All(s.Context(), &restored), "can decode the restored documents")

	s.Require().Len(restored, 1, "only the matching document is restored")
	// EqualValues, because an int that was inserted comes back from the server as
	// an int32.
	s.Assert().EqualValues(
		queryCase.matching[0].Value,
		restored[0].ID,
		"the restored document is the one the query matched, not the other one",
	)

	if queryCase.wantXType != nil {
		s.Assert().IsType(
			queryCase.wantXType,
			restored[0].X,
			"the matched value round-trips as the BSON type the query named",
		)
	}
}

// TestDumpForceTableScan dumps with --forceTableScan while documents are being
// inserted concurrently. The dump uses no index and takes no snapshot, so the
// result is only bounded: it must hold at least what existed when the dump
// started, and strictly fewer than exist once the inserts are stopped, which is
// what shows the dump read the collection while it was still growing.
func (s *DumpRestoreSuite) TestDumpForceTableScan() {
	const collName = "bar"

	// concurrentInsertCap bounds the background inserts. mongodump runs as a
	// subprocess that has to be compiled first, so an unbounded loop would insert
	// for as long as that takes and then make the restore pay for all of it.
	const concurrentInsertCap = 10_000

	testDB := s.database("force_table_scan")
	coll := testDB.Collection(collName)

	s.insertNamespacedDocs(coll)
	countBefore := s.docCount(coll)
	s.Require().Positive(countBefore, "the collection holds documents before the dump")

	// Captured here rather than called from the goroutine: s.Context() reads
	// s.T(), which testify swaps out when it moves to the next test method.
	ctx := s.Context()

	stop := make(chan struct{})
	stopped := false
	stopInserts := func() {
		if !stopped {
			close(stop)
			stopped = true
		}
	}

	var inserter sync.WaitGroup
	var insertErr error
	var hitCap bool
	inserter.Add(1)

	// Any assertion inside the dump helper below can end this goroutine's
	// owner via runtime.Goexit, so the shutdown has to be deferred too or the
	// inserts run on into the next test.
	defer func() {
		stopInserts()
		inserter.Wait()
	}()

	go func() {
		defer inserter.Done()

		for i := range concurrentInsertCap {
			select {
			case <-stop:
				return
			default:
			}

			if _, err := coll.InsertOne(ctx, bson.D{{"concurrent", i}}); err != nil {
				insertErr = err

				return
			}
		}

		hitCap = true
	}()

	var countAfter int64
	s.withBSONMongodump(func(dir string) {
		stopInserts()
		inserter.Wait()
		s.Require().NoError(insertErr, "the background inserts all succeeded")
		// The upper bound below relies on the inserts still being in flight when
		// they are stopped. Running out first is not a wrong answer to assert
		// against, it means the setup stopped exercising what the test is about,
		// so say so plainly rather than letting the test pass on a degenerate run.
		s.Require().False(
			hitCap,
			"the background inserts were still running when the dump finished; "+
				"raise concurrentInsertCap",
		)

		countAfter = s.docCount(coll)
		s.Require().Greater(
			countAfter,
			countBefore,
			"the concurrent inserts landed while the dump was running",
		)

		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a --forceTableScan dump")
	}, "--db", testDB.Name(), "--forceTableScan")

	restored := s.docCount(coll)
	s.Assert().GreaterOrEqual(
		restored,
		countBefore,
		"the dump holds at least the documents that existed when it started",
	)
	s.Assert().Less(
		restored,
		countAfter,
		"the dump missed documents inserted after it read past them, so it really did "+
			"read the collection while it was still growing",
	)
}

// TestDumpStorageEngineOptions round-trips a collection created with
// storage-engine options, and checks that the options themselves survive rather
// than only that the dump and restore succeeded.
func (s *DumpRestoreSuite) TestDumpStorageEngineOptions() {
	const collName = "testcoll"

	testDB := s.database("storage_engine_options")

	storageEngine := bson.D{
		{"wiredTiger", bson.D{{"configString", "block_compressor=zlib"}}},
	}
	s.createCollection(
		testDB,
		collName,
		mopt.CreateCollection().SetStorageEngine(storageEngine),
	)

	storageEngineBefore := s.collectionOption(testDB, collName, "storageEngine")
	s.Require().NotNil(
		storageEngineBefore,
		"the server reports the storage engine options that were asked for",
	)

	s.insertNamespacedDocs(testDB.Collection(collName))

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump with storage engine options")
		s.requireInserted(result, 1)
	}, "--db", testDB.Name())

	s.assertDocsCameFrom(testDB.Collection(collName), testDB.Name()+"."+collName)
	s.Assert().Equal(
		storageEngineBefore,
		s.collectionOption(testDB, collName, "storageEngine"),
		"the storage engine options survive the round trip",
	)
}
