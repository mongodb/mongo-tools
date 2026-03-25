package importexport

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type ImportExportSuite struct {
	suite.Suite
}

func TestImportExport(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ts := new(ImportExportSuite)
	suite.Run(t, ts)
}

func (s *ImportExportSuite) Client() *mongo.Client {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	client, err := sessionProvider.GetSession()
	s.Require().NoError(err, "no client available")

	return client
}

func (s *ImportExportSuite) RequireFCVAtLeast(wantFCV string) {
	fcv := testutil.GetFCV(s.Client())
	cmp, err := testutil.CompareFCV(fcv, wantFCV)
	s.Require().NoError(err, "get fcv")

	if cmp < 0 {
		s.T().Skipf("Requires server with FCV %s or later; found %v", wantFCV, fcv)
	}
}

func (s *ImportExportSuite) ServerVersion() db.Version {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	serverVersion, err := sessionProvider.ServerVersionArray()
	s.Require().NoError(err, "get server version")

	return serverVersion
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
