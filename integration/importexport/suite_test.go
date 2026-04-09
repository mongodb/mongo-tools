package importexport

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ImportExportSuite struct {
	sharedsuite.IntegrationSuite
}

func TestImportExport(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ts := new(ImportExportSuite)
	suite.Run(t, ts)
}

func (s *ImportExportSuite) ExportOptions() mongoexport.Options {
	toolOptions, err := testutil.GetToolOptions()
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

func (s *ImportExportSuite) ImportOptions(dbName, collName string) mongoimport.Options {
	ssl := testutil.GetSSLOptions()
	auth := testutil.GetAuthOptions()

	return mongoimport.Options{
		ToolOptions: &options.ToolOptions{
			General: &options.General{},
			SSL:     &ssl,
			Connection: &options.Connection{
				Host: "localhost",
				Port: db.DefaultTestPort,
			},
			Auth: &auth,
			URI:  &options.URI{},
			Namespace: &options.Namespace{
				DB:         dbName,
				Collection: collName,
			},
			WriteConcern: wcwrapper.Majority(),
		},
		InputOptions: &mongoimport.InputOptions{
			ParseGrace: "stop",
		},
		IngestOptions: &mongoimport.IngestOptions{
			Mode: "insert",
		},
	}
}

func (s *ImportExportSuite) importCollection(
	ns *options.Namespace,
	filePath string,
	ingestOpts mongoimport.IngestOptions,
) error {
	toolOptions, err := testutil.GetToolOptions()
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

func (s *ImportExportSuite) exportCollectionToFile(ns *options.Namespace) string {
	exportFile, err := os.CreateTemp(s.T().TempDir(), "export-*.json")
	s.Require().NoError(err)
	exportToolOptions, err := testutil.GetToolOptions()
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

func (s *ImportExportSuite) recreateWithValidator(coll *mongo.Collection, validator any) {
	s.Require().NoError(coll.Database().Drop(s.Context()))
	s.Require().NoError(coll.Database().CreateCollection(
		s.Context(),
		coll.Name(),
		mopt.CreateCollection().SetValidator(validator),
	))
}

func (s *ImportExportSuite) assertValidationError(err error, msg string) {
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
