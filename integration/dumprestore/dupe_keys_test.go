package dumprestore

import (
	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	duplicateKeysDocCount = 50

	// The field added after the dump is taken, so the test can tell a document
	// that survived from one that mongorestore overwrote.
	survivorMarker = "survived"
)

var duplicateKeysRemovedIDs = []int{0, 5, 6, 9, 12, 27, 40, 46, 47, 49}

// TestRestoreDuplicateKeys restores a dump over a collection that still holds
// most of the dumped documents. The documents that are already present produce
// duplicate key errors, which mongorestore reports as failures without failing
// the restore, while the missing documents are inserted.
func (s *DumpRestoreSuite) TestRestoreDuplicateKeys() {
	s.Run("default batch size", s.testDuplicateKeysDefaultBatchSize)
	s.Run("batch size of 1", s.testDuplicateKeysBatchSizeOne)
	s.Run("batch size that leaves a partial batch", s.testDuplicateKeysPartialBatch)
	s.Run("batch size larger than the dump", s.testDuplicateKeysLargeBatchSize)
}

func (s *DumpRestoreSuite) testDuplicateKeysDefaultBatchSize() {
	s.assertDuplicateKeyRestoreFillsGaps("duplicate_keys_default")
}

func (s *DumpRestoreSuite) testDuplicateKeysBatchSizeOne() {
	s.assertDuplicateKeyRestoreFillsGaps(
		"duplicate_keys_batch_1",
		mongorestore.BulkBufferSizeOption, "1",
	)
}

// testDuplicateKeysPartialBatch uses a batch size that does not divide the
// document count, so the last batch is flushed by the end of the input rather
// than by reaching the batch size. That is a different code path from a batch
// that fills up, and every batch size the other cases use divides evenly.
func (s *DumpRestoreSuite) testDuplicateKeysPartialBatch() {
	s.assertDuplicateKeyRestoreFillsGaps(
		"duplicate_keys_batch_7",
		mongorestore.BulkBufferSizeOption, "7",
	)
}

// testDuplicateKeysLargeBatchSize uses a batch size above the document count so
// every document lands in a single batch. The value is deliberately not the
// default of 1000, which the first case already covers.
func (s *DumpRestoreSuite) testDuplicateKeysLargeBatchSize() {
	s.assertDuplicateKeyRestoreFillsGaps(
		"duplicate_keys_batch_51",
		mongorestore.BulkBufferSizeOption, "51",
	)
}

func (s *DumpRestoreSuite) assertDuplicateKeyRestoreFillsGaps(
	dbName string,
	restoreArgs ...string,
) {
	testDB := s.database(dbName)
	coll := s.createDuplicateKeysFixture(testDB)

	s.withBSONMongodump(func(dir string) {
		s.removeAndMarkDocuments(coll)

		result := s.runRestore(append(restoreArgs, dir)...)
		s.Require().NoError(result.Err, "restoring over existing documents succeeds")
		s.Require().EqualValues(
			duplicateKeysDocCount,
			result.Successes+result.Failures,
			"every dumped document is accounted for as an insert or a failure",
		)
		s.Assert().EqualValues(
			len(duplicateKeysRemovedIDs),
			result.Successes,
			"only the missing documents are inserted",
		)
		s.Assert().EqualValues(
			duplicateKeysDocCount-len(duplicateKeysRemovedIDs),
			result.Failures,
			"every already-present document is reported as a duplicate key failure",
		)
	}, "--db", testDB.Name())

	s.Assert().EqualValues(
		duplicateKeysDocCount,
		s.docCount(coll),
		"the removed documents are restored",
	)
	s.assertSurvivorsUntouched(coll)
}

func (s *DumpRestoreSuite) createDuplicateKeysFixture(testDB *mongo.Database) *mongo.Collection {
	coll := testDB.Collection("duplicates")

	docs := make([]any, duplicateKeysDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the fixture documents")

	return coll
}

// removeAndMarkDocuments deletes the documents whose restoration is under test
// and marks every remaining one. The marker is added after the dump was taken,
// so it is absent from the dump: if mongorestore overwrote an existing document
// instead of reporting a duplicate key, the marker would disappear.
func (s *DumpRestoreSuite) removeAndMarkDocuments(coll *mongo.Collection) {
	deleted, err := coll.DeleteMany(
		s.Context(),
		bson.D{{"_id", bson.D{{"$in", duplicateKeysRemovedIDs}}}},
	)
	s.Require().NoError(err, "can delete the fixture documents")
	s.Require().EqualValues(
		len(duplicateKeysRemovedIDs),
		deleted.DeletedCount,
		"the expected documents are deleted",
	)

	updated, err := coll.UpdateMany(
		s.Context(),
		bson.D{},
		bson.D{{"$set", bson.D{{survivorMarker, true}}}},
	)
	s.Require().NoError(err, "can mark the surviving documents")
	s.Require().EqualValues(
		duplicateKeysDocCount-len(duplicateKeysRemovedIDs),
		updated.ModifiedCount,
		"every surviving document is marked",
	)
}

func (s *DumpRestoreSuite) assertSurvivorsUntouched(coll *mongo.Collection) {
	removed := map[int]bool{}
	for _, id := range duplicateKeysRemovedIDs {
		removed[id] = true
	}

	var restored []struct {
		ID       int  `bson:"_id"`
		Survived bool `bson:"survived"`
	}
	cursor, err := coll.Find(s.Context(), bson.D{})
	s.Require().NoError(err, "can read the restored documents")
	s.Require().NoError(cursor.All(s.Context(), &restored), "can decode the restored documents")

	for _, doc := range restored {
		if removed[doc.ID] {
			s.Assert().False(
				doc.Survived,
				"document %d comes from the dump, which was taken before the marker was added",
				doc.ID,
			)
		} else {
			s.Assert().True(
				doc.Survived,
				"document %d was left alone rather than overwritten",
				doc.ID,
			)
		}
	}
}
