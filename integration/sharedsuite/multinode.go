package sharedsuite

import (
	"net/url"
	"slices"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
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

// SecondaryHosts returns the hosts of every secondary in the replica set,
// including the unelectable ones the server reports as passives: a passive is
// still a secondary, holding data and serving reads, it just cannot be elected
// primary.
func (s *IntegrationSuite) SecondaryHosts() []string {
	_, secondaries := s.replicaSetHosts()
	s.Require().NotEmpty(secondaries, "the replica set has at least one secondary")

	return secondaries
}

// replicaSetHosts returns the primary's host and the hosts of every secondary,
// as the replica set itself names them.
func (s *IntegrationSuite) replicaSetHosts() (string, []string) {
	status := s.replicaSetStatus()
	s.Require().NotEmpty(status.Primary, "the replica set has a primary")

	secondaries := make([]string, 0, len(status.Hosts)+len(status.Passives))
	for _, host := range slices.Concat(status.Hosts, status.Passives) {
		if host != status.Primary {
			secondaries = append(secondaries, host)
		}
	}

	return status.Primary, secondaries
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

// AwaitCollectionOnSecondary blocks until the secondary has applied the
// collection's creation, so that a tool reading the secondary cannot race
// ahead of it. A write concern covering the whole set only tells us the write
// reached every member's journal; applying it is separate, and reads see
// applied state. A majority read concern makes the secondary wait until it
// has applied everything the set has majority-committed before answering,
// which turns the visibility check itself into the wait.
func (s *IntegrationSuite) AwaitCollectionOnSecondary(host, dbName, collName string) {
	secondary := s.DirectClient(host)
	defer func() {
		s.Require().NoError(
			secondary.Disconnect(s.Context()),
			"can disconnect from the secondary",
		)
	}()

	testDB := secondary.Database(
		dbName,
		mopt.Database().SetReadConcern(readconcern.Majority()),
	)
	names, err := testDB.ListCollectionNames(s.Context(), bson.D{{"name", collName}})
	s.Require().NoError(err, "can list the collections in %#q on %s", dbName, host)
	s.Require().Contains(
		names,
		collName,
		"%#q has been applied on the secondary %s",
		dbName+"."+collName,
		host,
	)
}

// DirectToolArgs returns the command-line arguments that point a tool at one
// specific node, so the tool connects directly rather than discovering the set and
// following it to the primary.
func (s *IntegrationSuite) DirectToolArgs(host string) []string {
	return s.toolArgsForHosts(
		[]string{host},
		url.Values{"directConnection": {"true"}},
		host,
	)
}

// directToolOptions builds tool options addressed at one node, keeping whatever
// SSL and auth configuration the test deployment needs.
func (s *IntegrationSuite) directToolOptions(host string) *options.ToolOptions {
	if uri, ok := s.configuredURIWithHosts(
		[]string{host},
		url.Values{"directConnection": {"true"}},
	); ok {
		opts, err := testopts.GetToolOptionsForURI(uri)
		s.Require().NoError(err, "can build tool options for %s", host)
		if opts.SSL.UseSSL {
			opts.SSL.SSLAllowInvalidHost = true
		}
		s.Require().True(opts.Direct, "the connection to %s is a direct one", host)

		return opts
	}

	opts, err := testopts.GetToolOptions()
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
