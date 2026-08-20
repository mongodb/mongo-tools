package dumprestore

import (
	"fmt"
	"path/filepath"

	"github.com/mongodb/mongo-tools/mongorestore"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// replacementDocCount differs from namespaceDocCount so that a restore which
// failed to drop the replacements is visible in the document count as well as
// in the document contents.
const replacementDocCount = 20

// TestRestoreDropWithData checks that --drop replaces the existing contents of
// every restored collection. The pre-dump data is swapped for a disjoint set of
// documents of a different size before the restore, so a restore that upserted
// by _id instead of dropping would leave both sets behind.
func (s *DumpRestoreSuite) TestRestoreDropWithData() {
	collNames := []string{"coll1", "coll2"}

	testDB := s.database("drop_with_data")
	for _, collName := range collNames {
		s.insertNamespacedDocs(testDB.Collection(collName))
	}

	s.withBSONMongodump(func(dir string) {
		for _, collName := range collNames {
			s.replaceCollectionContents(testDB.Collection(collName))
		}

		result := s.runRestore(mongorestore.DropOption, dir)
		s.Require().NoError(result.Err, "can restore with --drop")
		s.requireInserted(result, len(collNames))
	}, "--db", testDB.Name())

	for _, collName := range collNames {
		s.assertDocsCameFrom(
			testDB.Collection(collName),
			testDB.Name()+"."+collName,
		)
	}
}

// TestRestoreDropOneCollection checks that --drop scoped with --collection
// replaces only that collection, leaving its sibling and a second database
// holding the same collection names untouched.
func (s *DumpRestoreSuite) TestRestoreDropOneCollection() {
	const restoredCollName = "coll1"

	collNames := []string{restoredCollName, "coll2"}

	sourceDB := s.database("drop_one_collection_source")
	for _, collName := range collNames {
		s.insertNamespacedDocs(sourceDB.Collection(collName))
	}

	// A second database holding the same collection names, to catch a --drop
	// that reaches past the database it was scoped to.
	otherDB := s.database("drop_one_collection_other")

	s.withBSONMongodump(func(dir string) {
		for _, collName := range collNames {
			s.replaceCollectionContents(sourceDB.Collection(collName))
			s.replaceCollectionContents(otherDB.Collection(collName))
		}

		result := s.runRestore(
			mongorestore.DropOption,
			mongorestore.DBOption, sourceDB.Name(),
			mongorestore.CollectionOption, restoredCollName,
			filepath.Join(dir, sourceDB.Name(), restoredCollName+".bson"),
		)
		s.Require().NoError(result.Err, "can restore one collection with --drop")
		s.requireInserted(result, 1)
	}, "--db", sourceDB.Name())

	s.assertDocsCameFrom(
		sourceDB.Collection(restoredCollName),
		sourceDB.Name()+"."+restoredCollName,
	)

	s.assertOnlyReplacements(sourceDB.Collection("coll2"))
	for _, collName := range collNames {
		s.assertOnlyReplacements(otherDB.Collection(collName))
	}
}

// TestRestoreDropNonexistentDB checks that --drop against a database that does
// not exist is not an error and restores normally.
func (s *DumpRestoreSuite) TestRestoreDropNonexistentDB() {
	testDB := s.database("drop_nonexistent_db")
	s.insertNamespacedDocs(testDB.Collection("coll"))

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(mongorestore.DropOption, dir)
		s.Require().NoError(result.Err, "--drop with nothing to drop is not an error")
		s.requireInserted(result, 1)
	}, "--db", testDB.Name())

	s.assertDocsCameFrom(testDB.Collection("coll"), testDB.Name()+".coll")
}

// replaceCollectionContents swaps a collection's documents for a disjoint set,
// so that a restore which did not drop first leaves both sets behind and a
// restore which dropped leaves only the dumped ones.
func (s *DumpRestoreSuite) replaceCollectionContents(coll *mongo.Collection) {
	s.Require().NoError(
		coll.Drop(s.Context()),
		"can drop %#q before refilling it",
		namespaceOf(coll),
	)

	docs := make([]any, replacementDocCount)
	for i := range docs {
		docs[i] = bson.D{{"_id", replacementID(i, coll)}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert replacement documents into %#q", namespaceOf(coll))
}

func (s *DumpRestoreSuite) assertOnlyReplacements(coll *mongo.Collection) {
	var found []struct {
		ID string `bson:"_id"`
	}
	cursor, err := coll.Find(s.Context(), bson.D{})
	s.Require().NoError(err, "can read %#q", namespaceOf(coll))
	s.Require().NoError(cursor.All(s.Context(), &found), "can decode %#q", namespaceOf(coll))

	wantIDs := make([]string, replacementDocCount)
	for i := range wantIDs {
		wantIDs[i] = replacementID(i, coll)
	}

	gotIDs := make([]string, 0, len(found))
	for _, doc := range found {
		gotIDs = append(gotIDs, doc.ID)
	}

	s.Assert().ElementsMatch(
		wantIDs,
		gotIDs,
		"%#q still holds only the documents written after the dump",
		namespaceOf(coll),
	)
}

func replacementID(i int, coll *mongo.Collection) string {
	return fmt.Sprintf("replacement_%d_%s", i, namespaceOf(coll))
}
