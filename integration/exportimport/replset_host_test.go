package exportimport

import (
	"os"
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestExportImportOverReplicaSetHost exports and imports through a
// "<setName>/<seedlist>" host argument rather than a single host, which is how
// the tools are pointed at a replica set without a connection string.
func (s *ExportImportSuite) TestExportImportOverReplicaSetHost() {
	testtype.SkipUnlessTestType(s.T(), testtype.ReplSetTestType)

	const (
		collName = "bar"
		wantDocs = 100
	)

	dbName := s.DBName()
	coll := s.Client().Database(dbName).Collection(collName)

	docs := make([]any, 0, wantDocs)
	for i := range wantDocs {
		docs = append(docs, bson.D{{"_id", i}})
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the documents")

	hostArgs := s.ReplicaSetToolArgs()
	exportFile := filepath.Join(s.T().TempDir(), "export.json")

	s.exportWithArgs(hostArgs, exportFile, dbName, collName)
	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection before importing it")

	s.importWithArgs(hostArgs, exportFile, dbName, collName)

	count, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err, "can count the imported documents")
	s.Assert().EqualValues(
		wantDocs,
		count,
		"every document comes back from an export and import addressed to the replica set",
	)
}

func (s *ExportImportSuite) exportWithArgs(
	hostArgs []string,
	path, dbName, collName string,
) {
	args := append(append([]string{}, hostArgs...),
		"--db", dbName,
		"--collection", collName,
	)
	opts, err := mongoexport.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongoexport options")

	me, err := mongoexport.New(opts)
	s.Require().NoError(err, "mongoexport can connect")
	defer me.Close()

	out, err := os.Create(path)
	s.Require().NoError(err, "can create the export file")
	_, err = me.Export(out)
	s.Require().NoError(err, "mongoexport succeeds")
	s.Require().NoError(out.Close(), "can close the export file")
}

func (s *ExportImportSuite) importWithArgs(
	hostArgs []string,
	path, dbName, collName string,
) {
	args := append(append([]string{}, hostArgs...),
		"--db", dbName,
		"--collection", collName,
		"--file", path,
	)
	opts, err := mongoimport.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongoimport options")

	mi, err := mongoimport.New(opts)
	s.Require().NoError(err, "mongoimport can connect")
	defer mi.Close()

	_, failed, err := mi.ImportDocuments()
	s.Require().NoError(err, "mongoimport succeeds")
	s.Require().Zero(failed, "no document is rejected")
}
