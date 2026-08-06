package dumprestore

import (
	"strings"

	"github.com/mongodb/mongo-tools/common/db"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestRestoreLargeBulk checks that documents at the top of the size range a server
// accepts survive a dump and restore byte for byte.
//
// It does not reach the size-based batch split that TOOLS-939 was about, which the
// buffered bulk inserter makes at MAX_MESSAGE_SIZE_BYTES minus 1MB, roughly 47MB.
// Not because a document set that large would be slow -- 50MB round trips in under two
// seconds -- but because it could not show the split happening. The driver breaks
// an oversized InsertMany into several OP_MSG batches on its own, so a 50MB restore
// succeeds just the same with the inserter's byte limit raised out of the way. That
// split is covered by unit tests in common/db/buffered_bulk_test.go.
func (s *DumpRestoreSuite) TestRestoreLargeBulk() {
	// A shade under the largest document a server will accept, which is the only
	// size near this code that anything is actually built around: the BSON reader
	// rejects documents above it and both it and mongorestore size their buffers to
	// it. The payload is the bulk of the document, so a kilobyte of room is plenty
	// for the field name and the _id.
	const largeDocSize = db.MaxBSONSize - 1024
	const largeDocCount = 2

	testDB := s.database("large_bulk")
	coll := testDB.Collection("coll")

	payload := strings.Repeat("X", largeDocSize)
	docs := make([]any, largeDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", i}, {"data", payload}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the large documents")

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore documents at the maximum size")
		s.Assert().EqualValues(largeDocCount, result.Successes, "every document is inserted")
		s.Assert().EqualValues(0, result.Failures, "no document is rejected")
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
