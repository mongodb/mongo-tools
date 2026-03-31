package importexport

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportModeUpsertIDSubdoc verifies that --mode=upsert uses the full
// subdocument _id as the upsert key, preserving field order through a
// round-trip of export → upsert-replace → re-import.
func (s *ImportExportSuite) TestImportModeUpsertIDSubdoc() {
	const (
		dbName   = "mongoimport_upsert_subdoc_test"
		collName = "c"
	)

	client := s.newClient(dbName)

	coll := client.Database(dbName).Collection(collName)
	ns := &options.Namespace{DB: dbName, Collection: collName}

	origDocs := subdocIDDocs("string")
	insertDocs := make([]any, len(origDocs))
	for i, d := range origDocs {
		insertDocs[i] = d
	}
	_, err := coll.InsertMany(s.T().Context(), insertDocs)
	s.Require().NoError(err)

	exportedFile := s.exportCollectionToFile(ns)
	str2File := writeSubdocIDFile(s.T(), "str2")

	s.Run("upsert with replacement data updates all docs in place", func() {
		s.Require().NoError(
			s.importCollection(ns, str2File, mongoimport.IngestOptions{Mode: "upsert"}),
		)
		n, err := coll.CountDocuments(s.T().Context(), bson.D{})
		s.Require().NoError(err)
		s.EqualValues(20, n, "count should be unchanged after upsert")
		n, err = coll.CountDocuments(s.T().Context(), bson.D{{"x", "str2"}})
		s.Require().NoError(err)
		s.EqualValues(20, n, "all docs should have x=str2 after upsert")
	})

	s.Run("re-import original export reverts all docs", func() {
		s.Require().NoError(
			s.importCollection(ns, exportedFile, mongoimport.IngestOptions{Mode: "upsert"}),
		)
		n, err := coll.CountDocuments(s.T().Context(), bson.D{})
		s.Require().NoError(err)
		s.EqualValues(20, n, "count should be unchanged after re-import")
		n, err = coll.CountDocuments(s.T().Context(), bson.D{{"x", "string"}})
		s.Require().NoError(err)
		s.EqualValues(20, n, "all docs should have x=string after re-import")
	})
}

func writeSubdocIDFile(t *testing.T, xFieldValue string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "subdoc-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	for _, doc := range subdocIDDocs(xFieldValue) {
		b, err := bson.MarshalExtJSON(doc, true, false)
		if err != nil {
			t.Fatalf("marshal doc: %v", err)
		}
		if _, err = f.Write(b); err != nil {
			t.Fatalf("write doc: %v", err)
		}
		if _, err = f.Write([]byte("\n")); err != nil {
			t.Fatalf("write newline: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	return f.Name()
}

func subdocIDDocs(xFieldValue string) []bson.D {
	docs := make([]bson.D, 0, 20)
	for i := range 4 {
		for j := range 5 {
			docs = append(docs, bson.D{
				{"_id", bson.D{
					{"a", i},
					{"b", bson.A{0, 1, 2, bson.D{{"c", j}, {"d", "foo"}}}},
					{"e", "bar"},
				}},
				{"x", xFieldValue},
			})
		}
	}
	return docs
}
