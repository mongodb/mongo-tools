package importexport

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportModeUpsertIDSubdoc verifies that --mode=upsert uses the full
// subdocument _id as the upsert key, preserving field order through a
// round-trip of export → upsert-replace → re-import.
func TestImportModeUpsertIDSubdoc(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	const (
		dbName   = "mongoimport_upsert_subdoc_test"
		collName = "c"
	)

	client := newTestClient(t, dbName)

	coll := client.Database(dbName).Collection(collName)
	ns := &options.Namespace{DB: dbName, Collection: collName}

	origDocs := subdocIDDocs("string")
	insertDocs := make([]any, len(origDocs))
	for i, d := range origDocs {
		insertDocs[i] = d
	}
	_, err := coll.InsertMany(t.Context(), insertDocs)
	require.NoError(t, err)

	exportedFile := exportCollectionToFile(t, ns)
	str2File := writeSubdocIDFile(t, "str2")

	t.Run("upsert with replacement data updates all docs in place", func(t *testing.T) {
		require.NoError(
			t,
			importCollection(t, ns, str2File, mongoimport.IngestOptions{Mode: "upsert"}),
		)
		n, err := coll.CountDocuments(t.Context(), bson.D{})
		require.NoError(t, err)
		assert.EqualValues(t, 20, n, "count should be unchanged after upsert")
		n, err = coll.CountDocuments(t.Context(), bson.D{{"x", "str2"}})
		require.NoError(t, err)
		assert.EqualValues(t, 20, n, "all docs should have x=str2 after upsert")
	})

	t.Run("re-import original export reverts all docs", func(t *testing.T) {
		require.NoError(
			t,
			importCollection(t, ns, exportedFile, mongoimport.IngestOptions{Mode: "upsert"}),
		)
		n, err := coll.CountDocuments(t.Context(), bson.D{})
		require.NoError(t, err)
		assert.EqualValues(t, 20, n, "count should be unchanged after re-import")
		n, err = coll.CountDocuments(t.Context(), bson.D{{"x", "string"}})
		require.NoError(t, err)
		assert.EqualValues(t, 20, n, "all docs should have x=string after re-import")
	})
}

func writeSubdocIDFile(t *testing.T, xFieldValue string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "subdoc-*.json")
	require.NoError(t, err)
	for _, doc := range subdocIDDocs(xFieldValue) {
		b, err := bson.MarshalExtJSON(doc, true, false)
		require.NoError(t, err)
		_, err = f.Write(b)
		require.NoError(t, err)
		_, err = f.Write([]byte("\n"))
		require.NoError(t, err)
	}
	require.NoError(t, f.Close())
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
