package sharedsuite

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testopts"
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

// AwaitCollectionOnSecondary blocks until the secondary has applied the collection's creation, so
// that a tool reading the secondary cannot race ahead of it. A write concern covering the whole set
// only tells us the write reached every member's journal; applying it is separate, and reads see
// applied state.
//
// The wait compares the secondary's own applied opTime against the set's last-committed opTime. The
// caller writes with a write concern covering the whole set, so the commit point read here already
// includes the write, and a node that has applied through it has the collection in its catalog.
//
// Waiting on optimes rather than on a majority read is what makes this work on a
// disaggregated-storage (DSC) cluster. A DSC cluster's committed snapshot comes from the log
// service rather than from each node's applied state, so a majority read on a DSC secondary can
// return before that node has applied the DDL to its own catalog, and listCollections reads only
// the node's own catalog.
func (s *IntegrationSuite) AwaitCollectionOnSecondary(host, dbName, collName string) {
	secondary := s.DirectClient(host)
	defer func() {
		s.Require().NoError(
			secondary.Disconnect(s.Context()),
			"can disconnect from the secondary",
		)
	}()

	s.awaitAppliedPast(secondary, s.lastCommittedOpTime(), host)

	testDB := secondary.Database(dbName)
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

// awaitAppliedPast blocks until the node's own applied opTime reaches the given one, polling its
// replSetGetStatus. Polling optimes is the same wait the server's own test harness makes
// (ReplSetTest.awaitReplication): there is no command that blocks until a node applies, and unlike
// a majority read it holds on every cluster type the suite runs against.
func (s *IntegrationSuite) awaitAppliedPast(node *mongo.Client, through db.OpTime, host string) {
	// The context bounds the status reads themselves, not just the loop: the clients the suite
	// builds set no socket timeout, so without this a secondary that stops replying would hang a
	// read past the deadline.
	ctx, cancel := context.WithTimeout(s.Context(), 30*time.Second)
	defer cancel()

	for {
		applied, err := appliedOpTime(ctx, node)
		// An opTime that is not before the target is at or past it.
		if err == nil && !db.OpTimeLessThan(applied, through) {
			return
		}
		if ctx.Err() != nil {
			s.Require().NoError(err, "can read the applied opTime of %s", host)
			s.Require().FailNowf(
				"timed out waiting for the secondary to apply",
				"the secondary %s did not apply through %s within 30s; its last applied opTime was %s",
				host, through, applied,
			)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// lastCommittedOpTime returns the set's commit point as the primary sees it.  The client the suite
// builds connects to the whole set, so the command reaches the primary, whose view of the commit
// point is the most current.
func (s *IntegrationSuite) lastCommittedOpTime() db.OpTime {
	client := s.Client()
	defer func() {
		s.Require().NoError(
			client.Disconnect(s.Context()),
			"can disconnect after reading the commit point",
		)
	}()

	var status replSetOptimes
	err := client.Database("admin").
		RunCommand(s.Context(), bson.D{{"replSetGetStatus", 1}}).
		Decode(&status)
	s.Require().NoError(err, "can read the replica set's commit point")

	return status.Optimes.LastCommittedOpTime.opTime()
}

// appliedOpTime returns the answering node's own applied opTime from its replSetGetStatus.
func appliedOpTime(ctx context.Context, node *mongo.Client) (db.OpTime, error) {
	var status replSetOptimes
	err := node.Database("admin").
		RunCommand(ctx, bson.D{{"replSetGetStatus", 1}}).
		Decode(&status)
	if err != nil {
		return db.OpTime{}, fmt.Errorf("reading the applied opTime: %w", err)
	}

	return status.Optimes.AppliedOpTime.opTime(), nil
}

// replSetOptimes is the part of the replSetGetStatus response these waits need: the set's
// last-committed opTime and the answering node's own applied opTime.
type replSetOptimes struct {
	Optimes struct {
		AppliedOpTime       rsOptTime `bson:"appliedOpTime"`
		LastCommittedOpTime rsOptTime `bson:"lastCommittedOpTime"`
	} `bson:"optimes"`
}

// rsOptTime decodes the {t, ts} form an optime takes in replSetGetStatus. db.OpTime itself carries
// no bson tags because it is built from oplog entries instead, so the wait decodes into this and
// converts.
type rsOptTime struct {
	Term      *int64         `bson:"t"`
	Timestamp bson.Timestamp `bson:"ts"`
}

func (o rsOptTime) opTime() db.OpTime {
	return db.OpTime{Timestamp: o.Timestamp, Term: o.Term}
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
