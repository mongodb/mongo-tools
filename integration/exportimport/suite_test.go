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
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ExportImportSuite struct {
	sharedsuite.IntegrationSuite
}

// crossCluster is the source/target pair a round-trip test runs between. The wrapper that produces
// it lives in sharedsuite so the same orientation logic serves dump/restore and export/import.
type crossCluster = sharedsuite.CrossCluster

func TestImportExport(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ts := new(ExportImportSuite)
	suite.Run(t, ts)
}

// ExportOptionsForURI builds mongoexport options pointing at the given cluster, or at the default
// localhost:DefaultTestPort when uri is empty.
func (s *ExportImportSuite) ExportOptionsForURI(uri string) mongoexport.Options {
	toolOptions, err := testopts.GetToolOptionsForURI(uri)
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

// ImportOptionsForURI builds mongoimport options pointing at the given cluster, or at the default
// localhost:DefaultTestPort when uri is empty.
func (s *ExportImportSuite) ImportOptionsForURI(
	uri, dbName, collName string,
) mongoimport.Options {
	toolOptions, err := testopts.GetToolOptionsForURI(uri)
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

// importCollectionForURI imports filePath into ns against a specific cluster, or against the
// default localhost:DefaultTestPort when uri is empty.
func (s *ExportImportSuite) importCollectionForURI(
	uri string,
	ns *options.Namespace,
	filePath string,
	ingestOpts mongoimport.IngestOptions,
) error {
	toolOptions, err := testopts.GetToolOptionsForURI(uri)
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

// exportCollectionToFileForURI exports ns to a temp file against a specific cluster, or against the
// default localhost:DefaultTestPort when uri is empty.
func (s *ExportImportSuite) exportCollectionToFileForURI(
	uri string,
	ns *options.Namespace,
) string {
	exportFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	exportToolOptions, err := testopts.GetToolOptionsForURI(uri)
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
