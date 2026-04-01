package suite

import (
	"context"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testutil"
	testifySuite "github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
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

var systemDBs = mapset.NewSet("admin", "local", "config")

// BeforeTest drops all user databases before each test so every test starts
// with a clean slate. System databases (admin, local, config) are preserved.
func (s *IntegrationSuite) BeforeTest(_, _ string) {
	client := s.Client()
	defer client.Disconnect(context.Background()) //nolint:errcheck

	names, err := client.ListDatabaseNames(context.Background(), bson.D{})
	if err != nil {
		return
	}

	for _, name := range names {
		if !systemDBs.Contains(name) {
			_ = client.Database(name).Drop(context.Background())
		}
	}
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
