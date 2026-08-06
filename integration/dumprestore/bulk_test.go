package dumprestore

import (
	"strings"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	// A shade under the largest document a server will accept, which is the only
	// size near this code that anything is actually built around: the BSON reader
	// rejects documents above it and both it and mongorestore size their buffers to
	// it. The payload is the bulk of the document, so a kilobyte of room is plenty
	// for the field name and the _id.
	largeDocSize = db.MaxBSONSize - 1024

	// Two documents, so the fixture stays under the buffered bulk inserter's byte
	// limit and reaches the server as a single insert. Three would cross it.
	largeDocCount = 2
)

// TestRestoreLargeBulk checks that documents at the top of the size range a server
// accepts survive a dump and restore byte for byte, and that mongorestore sends
// them in one insert rather than one per document.
//
// It does not reach the size-based batch split that TOOLS-939 was about, which the
// buffered bulk inserter makes at MAX_MESSAGE_SIZE_BYTES minus 1MB, roughly 47MB.
// Not because a fixture that large would be slow -- 50MB round trips in under two
// seconds -- but because it could not show the split happening. The driver breaks
// an oversized InsertMany into several OP_MSG batches on its own, so a 50MB restore
// succeeds just the same with the inserter's byte limit raised out of the way. That
// split is covered by unit tests in common/db/buffered_bulk_test.go.
func (s *DumpRestoreSuite) TestRestoreLargeBulk() {
	testDB := s.database("large_bulk")
	coll := testDB.Collection("coll")

	payload := strings.Repeat("X", largeDocSize)
	docs := make([]any, largeDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", i}, {"data", payload}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the large fixture documents")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		var result mongorestore.Result
		inserts := s.countInsertCommands(testDB, func() {
			result = s.runRestore(dir)
		})
		s.Require().NoError(result.Err, "can restore documents at the maximum size")
		s.Assert().EqualValues(largeDocCount, result.Successes, "every document is inserted")
		s.Assert().EqualValues(0, result.Failures, "no document is rejected")
		s.Assert().EqualValues(
			1,
			inserts,
			"the documents are batched into one insert rather than sent one per write",
		)
	}, "--db", testDB.Name())

	s.Assert().EqualValues(largeDocCount, s.docCount(coll), "every document is restored")

	var restored []struct {
		ID   int    `bson:"_id"`
		Data string `bson:"data"`
	}
	cursor, err := coll.Find(s.Context(), bson.D{})
	s.Require().NoError(err, "can read the restored documents")
	s.Require().NoError(cursor.All(s.Context(), &restored), "can decode the restored documents")

	for _, doc := range restored {
		s.Assert().Equal(
			payload,
			doc.Data,
			"document %d is restored byte for byte",
			doc.ID,
		)
	}
}

// countInsertCommands returns the number of insert commands the server ran against
// testDB while body ran.
//
// The count comes from the database profiler rather than from serverStatus so that
// only this database's operations are counted, and because serverStatus counts
// inserted documents rather than the commands that carried them, which is the whole
// distinction this test is drawing.
func (s *DumpRestoreSuite) countInsertCommands(testDB *mongo.Database, body func()) int64 {
	// Profiling has to be off to drop the collection it writes to, and entries left
	// from earlier operations on this database would be counted along with ours.
	s.setProfilingLevel(testDB, profilingOff)
	s.Require().NoError(
		testDB.Collection("system.profile").Drop(s.Context()),
		"can clear the profile collection in %#q",
		testDB.Name(),
	)
	s.setProfilingLevel(testDB, profileEverything)
	defer s.setProfilingLevel(testDB, profilingOff)

	body()

	count, err := testDB.Collection("system.profile").
		CountDocuments(s.Context(), bson.D{{"op", "insert"}})
	s.Require().NoError(err, "can count the profiled insert commands in %#q", testDB.Name())

	return count
}

const (
	profilingOff      = 0
	profileEverything = 2
)

func (s *DumpRestoreSuite) setProfilingLevel(testDB *mongo.Database, level int) {
	err := testDB.RunCommand(s.Context(), bson.D{{"profile", level}}).Err()
	s.Require().NoError(
		err,
		"can set the profiling level on %#q to %d",
		testDB.Name(),
		level,
	)
}
