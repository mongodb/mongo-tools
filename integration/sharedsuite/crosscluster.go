package sharedsuite

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// CrossCluster names the two clusters a round-trip test runs between and which
// side each role maps to. The tool that produces the data and the test's setup
// use the source; the tool that consumes the data and the post-run assertions
// use the target.
type CrossCluster struct {
	Source, Target       *mongo.Client
	SourceURI, TargetURI string
}

// systemDatabaseNames are the databases mongod manages itself, which a test must
// never drop.
var systemDatabaseNames = []string{"admin", "config", "local"}

// SkipForCrossCluster skips the test when a second cluster is configured, because the
// test assumes source and target are the same cluster. reason says what that assumption is.
func (s *IntegrationSuite) SkipForCrossCluster(reason string) {
	if os.Getenv(testopts.URIEnvVar2) != "" {
		s.T().Skip(reason)
	}
}

// WithCrossCluster runs a round-trip test body once per source/target
// orientation. In single-cluster mode it runs once, with source and target both
// the primary cluster. In cross-cluster mode it runs twice, once with each
// cluster as the source, so a test exercises the boundary in both directions.
func (s *IntegrationSuite) WithCrossCluster(body func(CrossCluster)) {
	primaryURI := os.Getenv(testopts.URIEnvVar)
	secondURI := os.Getenv(testopts.URIEnvVar2)

	if secondURI == "" {
		session, err := testutil.GetBareSession(s.T())
		s.Require().NoError(err, "can connect to the server")
		s.DropUserDatabases(session)
		body(CrossCluster{
			Source:    session,
			Target:    session,
			SourceURI: primaryURI,
			TargetURI: primaryURI,
		})
		return
	}

	for _, orientation := range []struct {
		source, target string
	}{
		{primaryURI, secondURI},
		{secondURI, primaryURI},
	} {
		s.Run(
			fmt.Sprintf(
				"from %s into %s",
				URILabel(orientation.source),
				URILabel(orientation.target),
			),
			func() {
				source, err := testutil.GetBareSessionForURI(s.T(), orientation.source)
				s.Require().NoError(err, "can connect to the source cluster")
				target, err := testutil.GetBareSessionForURI(s.T(), orientation.target)
				s.Require().NoError(err, "can connect to the target cluster")

				s.DropUserDatabases(source)
				s.DropUserDatabases(target)

				body(CrossCluster{
					Source:    source,
					Target:    target,
					SourceURI: orientation.source,
					TargetURI: orientation.target,
				})
			},
		)
	}
}

// DropUserDatabases drops every user (non-system) database on cluster so a test
// body starts from a clean slate. The two cluster orientations share clusters,
// so this keeps a later orientation from tripping over what an earlier one left
// behind.
func (s *IntegrationSuite) DropUserDatabases(cluster *mongo.Client) {
	names, err := cluster.ListDatabaseNames(s.Context(), bson.D{})
	s.Require().NoError(err, "can list the databases")

	for _, name := range names {
		if slices.Contains(systemDatabaseNames, name) {
			continue
		}
		s.Require().NoError(cluster.Database(name).Drop(s.Context()), "can drop database %#q", name)
	}
}

// URILabel returns a short human-readable name for a cluster URI, for use in test subtest names. An
// empty URI means the default localhost:DefaultTestPort.
func URILabel(uri string) string {
	if uri == "" {
		return "localhost:" + testopts.DefaultTestPort
	}

	cs, err := connstring.ParseAndValidate(uri)
	if err != nil {
		return uri
	}
	return strings.Join(cs.Hosts, ",")
}
