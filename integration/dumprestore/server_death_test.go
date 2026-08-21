package dumprestore

import (
	"strconv"
	"time"

	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongodump"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestDumpFailsWhenServerDies checks that mongodump reports a read failure when the
// server it is reading from goes away mid-dump, rather than hanging, panicking, or
// claiming the dump succeeded.
//
// The server is one this test starts and kills itself, not the test deployment,
// which the rest of the suite needs.
//
// The suite's own test-type gate is all this needs: it depends on nothing but a
// mongod binary on disk, which every integration variant has.
func (s *DumpRestoreSuite) TestDumpFailsWhenServerDies() {
	const (
		dbName   = "foo"
		collName = "bar"
	)

	mongod := s.StartThrowawayMongod()
	coll := mongod.Client.Database(dbName).Collection(collName)
	s.seedSlowDumpData(coll)

	// The dump reads every document through a $where that sleeps, so it is still
	// running when the server dies. Without that it would finish first and there
	// would be nothing to interrupt.
	ranFor, dumpErr := s.dumpWhileKillingServer(
		mongod,
		"--out", s.T().TempDir(),
		"--db", dbName,
		"--collection", collName,
		"--query", `{"$where": "sleep(25); return true;"}`,
	)

	// Without this, a dump that failed before the server was killed — because the
	// binary was missing a library, say, or the query was rejected — would satisfy
	// every assertion below.
	s.Require().Greater(
		ranFor,
		dumpHeadStart,
		"the dump was still running when the server was killed",
	)

	s.Require().NotNil(dumpErr, "mongodump fails when the server it is reading from dies")
	s.Require().Regexp(
		serverDiedErrors,
		dumpErr.Error(),
		"mongodump reports that it could not read from the server",
	)
}

// serverDiedErrors matches the errors mongodump can surface for a server that went
// away, depending on how far into the read it got and where the failure surfaced.
// It deliberately does not accept "connection refused": that is what a dump reports
// when it never reached the server at all, which is a different failure.
//
// "incomplete read of message header" is the driver's own wording for a connection
// that died partway through a wire message. Only that prefix is portable - the
// wrapped error after it is whatever the OS calls a reset connection, "connection
// reset by peer" on Linux and "wsarecv: An existing connection was forcibly closed
// by the remote host" on Windows.
const serverDiedErrors = `(?i)error reading from db|error reading collection|` +
	`connection closed|interrupted|socket was unexpectedly closed|` +
	`incomplete read of message header`

func (s *DumpRestoreSuite) seedSlowDumpData(coll *mongo.Collection) {
	docs := make([]any, slowDumpDocCount)
	for i := range docs {
		docs[i] = bson.D{{"x", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the documents into the throwaway mongod")
}

// slowDumpDocCount is high enough that the dump's sleeping $where keeps it running
// for far longer than it takes to kill the server underneath it.
const slowDumpDocCount = 1000

// dumpWhileKillingServer starts a dump against the throwaway mongod and kills it
// once the dump is under way. It returns whatever the dump reports, along with how
// long the dump itself ran, which is what tells a dump the kill interrupted apart
// from one that had already failed for some other reason.
func (s *DumpRestoreSuite) dumpWhileKillingServer(
	mongod *sharedsuite.ThrowawayMongod,
	dumpArgs ...string,
) (time.Duration, error) {
	opts, err := mongodump.ParseOptions(
		append(
			[]string{
				"--host", mongod.Host,
				"--serverSelectionTimeout", strconv.Itoa(serverSelectionTimeoutSeconds),
			},
			dumpArgs...,
		),
		"",
		"",
	)
	s.Require().NoError(err, "can parse the mongodump options")

	dump := &mongodump.MongoDump{
		ToolOptions:   opts.ToolOptions,
		InputOptions:  opts.InputOptions,
		OutputOptions: opts.OutputOptions,
	}
	s.Require().NoError(dump.Init(), "mongodump can connect to the throwaway mongod")

	killed := make(chan error, 1)
	go func() {
		time.Sleep(dumpHeadStart)
		killed <- mongod.Process.Kill()
	}()

	start := time.Now()
	dumpErr := dump.Dump()
	ranFor := time.Since(start)

	s.Require().NoError(<-killed, "the throwaway mongod is killed while the dump reads from it")

	return ranFor, dumpErr
}

// serverSelectionTimeoutSeconds keeps the dump from waiting out the driver's
// default. When the server dies the driver retries the read, and that retry sits in
// server selection waiting for a replacement that is never coming before the dump
// reports the failure it already had.
const serverSelectionTimeoutSeconds = 2

// dumpHeadStart is how long the dump is left running before the server is killed.
// The dump takes tens of seconds to finish, so this only has to be long enough for
// it to have started reading.
const dumpHeadStart = time.Second
