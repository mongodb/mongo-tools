package sharedsuite

import (
	"context"
	"strings"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testutil"
	testifySuite "github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type IntegrationSuite struct {
	testifySuite.Suite
}

func (s *IntegrationSuite) Context() context.Context {
	return s.T().Context()
}

// Client creates a new MongoDB client. The caller is responsible for cleanup.
func (s *IntegrationSuite) Client() *mongo.Client {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	client, err := sessionProvider.GetSession()
	s.Require().NoError(err, "no client available")

	return client
}

// DBName returns a valid MongoDB database name for the current test. It
// replaces slashes (introduced by testify suite nesting) with underscores and
// truncates the result to 63 bytes, the maximum MongoDB allows. An optional
// prefix (e.g. a system-collection prefix) is prepended before truncation so
// the total length never exceeds the limit.
func (s *IntegrationSuite) DBName(prefix ...string) string {
	p := strings.Join(prefix, "")
	name := strings.ReplaceAll(s.T().Name(), "/", "_")
	maxLen := 63 - len(p)
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return p + name
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
