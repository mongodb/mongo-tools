package exportimport

import (
	"os"
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportToSecondaryFails checks that an import addressed at a secondary fails
// rather than reporting success for writes the secondary cannot take. The export
// it imports is taken from the primary, so the only thing that can fail is the
// write.
func (s *ExportImportSuite) TestImportToSecondaryFails() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	const (
		collName = "bar"
		wantDocs = 20
	)

	dbName := s.DBName()
	coll := s.Client().Database(dbName).Collection(collName)

	docs := make([]any, 0, wantDocs)
	for i := range wantDocs {
		docs = append(docs, bson.D{{"_id", i}, {"y", "abc"}})
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the documents")

	exportFile := filepath.Join(s.T().TempDir(), "export.json")
	s.exportWithArgs(s.DirectToolArgs(s.PrimaryHost()), exportFile, dbName, collName)

	contents, err := os.ReadFile(exportFile)
	s.Require().NoError(err, "can read the export file")
	s.Require().NotEmpty(contents, "the export from the primary is not empty")

	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection before importing it")

	args := append(s.DirectToolArgs(s.SecondaryHost()),
		"--db", dbName,
		"--collection", collName,
		"--file", exportFile,
	)
	opts, err := mongoimport.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongoimport options")

	mi, err := mongoimport.New(opts)
	s.Require().NoError(err, "mongoimport can connect to the secondary")
	defer mi.Close()

	_, _, err = mi.ImportDocuments()
	s.Require().NotNil(err, "mongoimport fails when it is pointed at a secondary")
	s.Require().Regexp(
		sharedsuite.NotWritablePrimary,
		err.Error(),
		"mongoimport fails because the secondary will not take the writes",
	)

	count, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err, "can count the documents in the collection")
	s.Assert().Zero(count, "the rejected import leaves the collection empty")
}
