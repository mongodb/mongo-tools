package sharedsuite

import (
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// NotWritablePrimary matches the error a replica set node gives for a write it
// cannot take. Servers before 5.0 word this as "not master".
const NotWritablePrimary = "not (primary|master)"

// PrimaryHost returns the host the replica set currently considers its primary.
func (s *IntegrationSuite) PrimaryHost() string {
	primary, _ := s.replicaSetHosts()

	return primary
}

// SecondaryHost returns the host of one of the replica set's secondaries. Which
// one is unspecified; the tests that need a secondary need any secondary.
func (s *IntegrationSuite) SecondaryHost() string {
	return s.SecondaryHosts()[0]
}

// SecondaryHosts returns the hosts of every secondary in the replica set.
func (s *IntegrationSuite) SecondaryHosts() []string {
	_, secondaries := s.replicaSetHosts()
	s.Require().NotEmpty(secondaries, "the replica set has at least one secondary")

	return secondaries
}

// replicaSetHosts returns the primary's host and the hosts of every secondary,
// as the replica set itself names them.
func (s *IntegrationSuite) replicaSetHosts() (string, []string) {
	// isMaster rather than hello because the servers this runs against include
	// versions older than 5.0, where hello does not exist.
	var isMaster struct {
		Primary string   `bson:"primary"`
		Hosts   []string `bson:"hosts"`
	}
	err := s.Client().Database("admin").
		RunCommand(s.Context(), bson.D{{"isMaster", 1}}).
		Decode(&isMaster)
	s.Require().NoError(err, "can ask the server about the replica set it belongs to")
	s.Require().NotEmpty(isMaster.Primary, "the replica set has a primary")

	secondaries := make([]string, 0, len(isMaster.Hosts))
	for _, host := range isMaster.Hosts {
		if host != isMaster.Primary {
			secondaries = append(secondaries, host)
		}
	}

	return isMaster.Primary, secondaries
}

// DirectClient connects to one specific node, so that what the test observes is
// that node's own state rather than whatever the set steers it to. The caller is
// responsible for disconnecting it.
func (s *IntegrationSuite) DirectClient(host string) *mongo.Client {
	sessionProvider, err := db.NewSessionProvider(*s.directToolOptions(host))
	s.Require().NoError(err, "can connect directly to %s", host)

	client, err := sessionProvider.GetSession()
	s.Require().NoError(err, "can get a session for %s", host)

	return client
}

// DirectToolArgs returns the command-line arguments that point a tool at one
// specific node. Without a replica set name in --host, the tools connect directly
// rather than discovering the set and following it to the primary.
func (s *IntegrationSuite) DirectToolArgs(host string) []string {
	args := append([]string{}, testutil.GetSSLArgs()...)
	if len(args) > 0 {
		// The replica set names its members by hostnames the test certificate was
		// not issued for.
		args = append(args, "--sslAllowInvalidHostnames")
	}
	args = append(args, testutil.GetAuthArgs()...)

	return append(args, "--host", host)
}

// directToolOptions builds tool options addressed at one node, keeping whatever
// SSL and auth configuration the test deployment needs.
func (s *IntegrationSuite) directToolOptions(host string) *options.ToolOptions {
	opts, err := testutil.GetToolOptions()
	s.Require().NoError(err, "can build tool options")

	// The URI has already been built from the default host and port, so it has to
	// be cleared for the new host to take effect.
	opts.URI = &options.URI{}
	opts.Connection.Host = host
	opts.Connection.Port = ""
	if opts.SSL.UseSSL {
		opts.SSL.SSLAllowInvalidHost = true
	}

	s.Require().NoError(opts.NormalizeOptionsAndURI(), "can normalize options for %s", host)
	s.Require().True(opts.Direct, "the connection to %s is a direct one", host)

	return opts
}
