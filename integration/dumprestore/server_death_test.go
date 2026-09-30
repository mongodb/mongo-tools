package dumprestore

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/mongodb/mongo-tools/release/platform"
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

	mongod := s.startThrowawayMongod()
	coll := mongod.client.Database(dbName).Collection(collName)
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

// throwawayMongod is a mongod this test owns, so it can be killed without
// disturbing the deployment the rest of the suite shares.
type throwawayMongod struct {
	process *os.Process
	host    string
	client  *mongo.Client
	stderr  *lockedBuffer
}

// lockedBuffer collects a subprocess's output for a failure message. os/exec fills it
// from a goroutine of its own, so reading it while the process is still running needs
// the lock.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.buffer.String()
}

func (s *DumpRestoreSuite) startThrowawayMongod() *throwawayMongod {
	binary := s.mongodBinary()
	dbPath := s.T().TempDir()
	port := s.freePort()

	// No auth and no TLS, whatever the deployment under test uses: nothing else
	// connects to this server, and the test builds its own arguments for it.
	cmd := exec.CommandContext(
		s.Context(),
		binary,
		"--port", port,
		"--dbpath", dbPath,
		"--bind_ip", "localhost",
	)

	// Kept so that a mongod which refuses to start can say why, instead of the test
	// only reporting that it never came up.
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr

	s.Require().NoError(cmd.Start(), "can start a mongod of our own from %s", binary)

	mongod := &throwawayMongod{
		process: cmd.Process,
		host:    net.JoinHostPort("localhost", port),
		stderr:  stderr,
	}
	s.T().Cleanup(func() {
		// The test kills this process itself; this is only for the paths where it
		// did not get that far.
		_ = mongod.process.Kill()
		_ = cmd.Wait()
	})

	mongod.client = s.waitForMongod(mongod)

	return mongod
}

// mongodBinary returns the path to a mongod this test can start. CI downloads one
// alongside the built tools and nothing tells the test where it is, so the default is
// the path relative to the package directory, which is where go test runs.
func (s *DumpRestoreSuite) mongodBinary() string {
	if fromEnv := os.Getenv(mongodBinaryEnvVar); fromEnv != "" {
		return fromEnv
	}

	binary := filepath.Join("..", "..", "bin", "mongod"+platform.GetLocalBinaryExt())
	if _, err := os.Stat(binary); err == nil {
		return binary
	}

	onPath, err := exec.LookPath("mongod")
	if err == nil {
		return onPath
	}

	s.T().Skipf(
		"skipping because this test starts a mongod of its own and there is none at %s,"+
			" none on the PATH, and %s is not set",
		binary,
		mongodBinaryEnvVar,
	)

	return ""
}

const mongodBinaryEnvVar = "TOOLS_TESTING_MONGOD_BINARY"

// freePort returns a port nothing is listening on, by taking one from the operating
// system and handing it straight back. Something else could claim it in between, in
// which case mongod fails to bind and the test reports that it never came up.
func (s *DumpRestoreSuite) freePort() string {
	listener, err := net.Listen("tcp", "localhost:0")
	s.Require().NoError(err, "can find a free port to start a mongod on")
	defer func() {
		s.Require().NoError(listener.Close(), "can release the port again")
	}()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	s.Require().NoError(err, "can read the port back out of the listener's address")

	return port
}

func (s *DumpRestoreSuite) waitForMongod(mongod *throwawayMongod) *mongo.Client {
	client := s.throwawayClient(mongod.host)
	s.Require().Eventually(
		func() bool { return client.Ping(s.Context(), nil) == nil },
		mongodStartupTimeout,
		mongodPollInterval,
		"the mongod started on %s comes up; its stderr was:\n%s",
		mongod.host,
		mongod.stderr,
	)
	s.T().Cleanup(func() {
		// The server this client is connected to is killed by the time the test
		// ends, so a failure to disconnect from it is not a failure of the test.
		_ = client.Disconnect(s.Context())
	})

	return client
}

const (
	mongodStartupTimeout = 30 * time.Second
	mongodPollInterval   = 100 * time.Millisecond
)

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
	mongod *throwawayMongod,
	dumpArgs ...string,
) (time.Duration, error) {
	opts, err := mongodump.ParseOptions(
		append(
			[]string{
				"--host", mongod.host,
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
		killed <- mongod.process.Kill()
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

// throwawayClient connects to a server that has neither auth nor TLS, whatever the
// deployment the suite otherwise talks to requires.
func (s *DumpRestoreSuite) throwawayClient(host string) *mongo.Client {
	toolOptions := &options.ToolOptions{
		Connection: &options.Connection{Host: host},
		Auth:       &options.Auth{},
		SSL:        &options.SSL{},
		Namespace:  &options.Namespace{},
		URI:        &options.URI{},
	}
	s.Require().NoError(
		toolOptions.NormalizeOptionsAndURI(),
		"can normalize options for the throwaway mongod",
	)

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	s.Require().NoError(err, "can build a session provider for the throwaway mongod")

	client, err := sessionProvider.GetSession()
	s.Require().NoError(err, "can get a session for the throwaway mongod")

	return client
}
