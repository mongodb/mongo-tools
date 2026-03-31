package suite

import (
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testutil"
	testifySuite "github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type IntegrationSuite struct {
	testifySuite.Suite
}

// Client creates a new MongoDB client. The caller is responsible for cleanup.
func (s *IntegrationSuite) Client() *mongo.Client {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	client, err := sessionProvider.GetSession()
	s.Require().NoError(err, "no client available")

	return client
}

func (s *IntegrationSuite) RequireFCVAtLeast(wantFCV string) {
	fcv := testutil.GetFCV(s.Client())
	cmp, err := testutil.CompareFCV(fcv, wantFCV)
	s.Require().NoError(err, "get fcv")

	if cmp < 0 {
		s.T().Skipf("Requires server with FCV %s or later; found %v", wantFCV, fcv)
	}
}

func (s *IntegrationSuite) ServerVersion() db.Version {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	serverVersion, err := sessionProvider.ServerVersionArray()
	s.Require().NoError(err, "get server version")

	return serverVersion
}
