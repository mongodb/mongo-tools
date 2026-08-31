package exportimport

import (
	"path/filepath"

	"github.com/mongodb/mongo-tools/common/testtype"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportRoutesWritesToThePrimary covers the two ways of addressing a replica
// set that have to end up writing to the primary: the primary's own host, and a
// set-name seedlist that names only a secondary, where the driver has to
// discover the primary for itself. An import aimed straight at a secondary is
// covered separately, by TestImportToSecondaryFails.
func (s *ExportImportSuite) TestImportRoutesWritesToThePrimary() {
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

	importFile := filepath.Join(s.T().TempDir(), "export.json")
	s.exportWithArgs(s.DirectToolArgs(s.PrimaryHost()), importFile, dbName, collName)
	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection before importing it")

	cases := []struct {
		name     string
		hostArgs []string
	}{
		{"the primary's own host", s.DirectToolArgs(s.PrimaryHost())},
		{
			"a set-name seedlist naming only a secondary",
			s.ReplicaSetToolArgsForHosts(s.SecondaryHost()),
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			s.importWithArgs(c.hostArgs, importFile, dbName, collName)

			count, err := coll.CountDocuments(s.Context(), bson.D{})
			s.Require().NoError(err, "can count the imported documents")
			s.Assert().EqualValues(
				wantDocs,
				count,
				"every document is imported through %s",
				c.name,
			)

			s.Require().NoError(
				coll.Drop(s.Context()),
				"can drop the collection before the next case imports into it",
			)
		})
	}
}
