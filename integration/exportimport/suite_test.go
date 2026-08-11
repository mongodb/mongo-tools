package exportimport

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ExportImportSuite struct {
	sharedsuite.IntegrationSuite
}

func TestImportExport(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ts := new(ExportImportSuite)
	suite.Run(t, ts)
}

func (s *ExportImportSuite) ExportOptions() mongoexport.Options {
	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)

	opts := mongoexport.Options{
		ToolOptions: toolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	}

	log.SetVerbosity(toolOptions.Verbosity)

	return opts
}

func (s *ExportImportSuite) ImportOptions(dbName, collName string) mongoimport.Options {
	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	toolOptions.Namespace.DB = dbName
	toolOptions.Namespace.Collection = collName

	return mongoimport.Options{
		ToolOptions: toolOptions,
		InputOptions: &mongoimport.InputOptions{
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{
			Mode: "insert",
		},
	}
}

func (s *ExportImportSuite) importCollection(
	ns *options.Namespace,
	filePath string,
	ingestOpts mongoimport.IngestOptions,
) error {
	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	toolOptions.Namespace = ns
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   toolOptions,
		InputOptions:  &mongoimport.InputOptions{File: filePath, ParseGrace: "stop"},
		IngestOptions: &ingestOpts,
	})
	if err != nil {
		return err
	}
	defer mi.Close()
	_, _, err = mi.ImportDocuments()
	return err
}

func (s *ExportImportSuite) exportCollectionToFile(ns *options.Namespace) string {
	exportFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	exportToolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	exportToolOptions.Namespace = ns
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()
	_, err = me.Export(exportFile)
	s.Require().NoError(err)
	s.Require().NoError(exportFile.Close())
	return exportFile.Name()
}

// exportJSONFile exports ns as canonical extended JSON to a temp file and
// returns its path. Unlike exportCollectionToFile it can produce a single JSON
// array rather than a document per line.
func (s *ExportImportSuite) exportJSONFile(ns *options.Namespace, jsonArray bool) string {
	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	toolOptions.Namespace = ns
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: toolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
			JSONArray:  jsonArray,
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	s.Require().NoError(err)
	defer me.Close()

	exportFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	_, err = me.Export(exportFile)
	s.Require().NoError(err)
	s.Require().NoError(exportFile.Close())
	return exportFile.Name()
}

type importFlags struct {
	jsonArray bool
	legacy    bool
}

// importJSONFile imports path into ns and returns the number of documents
// inserted, failing the test if the import errors or the server rejects a
// document.
func (s *ExportImportSuite) importJSONFile(
	ns *options.Namespace,
	path string,
	flags importFlags,
) uint64 {
	toolOptions, err := testopts.GetToolOptions()
	s.Require().NoError(err)
	toolOptions.Namespace = ns
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions: toolOptions,
		InputOptions: &mongoimport.InputOptions{
			File:       path,
			ParseGrace: "stop",
			JSONArray:  flags.jsonArray,
			Legacy:     flags.legacy,
		},
		IngestOptions: &mongoimport.IngestOptions{},
	})
	s.Require().NoError(err)
	defer mi.Close()
	imported, failed, err := mi.ImportDocuments()
	s.Require().NoError(err)
	s.Require().Zero(failed, "no document is rejected by the server")
	return imported
}

// docsSortedByID returns every document in coll as a bson.D, ordered by _id so
// that a comparison against the documents that went in is a total order.
func (s *ExportImportSuite) docsSortedByID(coll *mongo.Collection) []any {
	cursor, err := coll.Find(
		s.Context(),
		bson.D{},
		mopt.Find().SetSort(bson.D{{"_id", 1}}),
	)
	s.Require().NoError(err)
	defer cursor.Close(s.Context())

	var docs []any
	for cursor.Next(s.Context()) {
		var doc bson.D
		s.Require().NoError(cursor.Decode(&doc))
		docs = append(docs, doc)
	}
	s.Require().NoError(cursor.Err())
	return docs
}

func (s *ExportImportSuite) recreateWithValidator(coll *mongo.Collection, validator any) {
	s.Require().NoError(coll.Database().Drop(s.Context()))
	s.Require().NoError(coll.Database().CreateCollection(
		s.Context(),
		coll.Name(),
		mopt.CreateCollection().SetValidator(validator),
	))
}

func (s *ExportImportSuite) assertValidationError(err error, msg string) {
	var bwe mongo.BulkWriteException
	if s.Assert().ErrorAs(err, &bwe, msg) {
		s.Assert().NotEmpty(bwe.WriteErrors, "should have at least one write error")
		s.Assert().Equal(
			121,
			bwe.WriteErrors[0].Code,
			"should be DocumentValidationFailure (121)",
		)
	}
}
