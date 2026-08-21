//go:build integration || sharded

package dumprestore

import (
	"io"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/mongodb/mongo-tools/mongodump"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestDumpFailsWhenServerDies checks that mongodump reports a read failure when
// the connection it is reading through dies mid-dump, rather than hanging,
// panicking, or claiming the dump succeeded.
//
// The death is simulated rather than real: the dump reads the deployment
// through a TCP proxy this test owns, and once the dump is under way the test
// cuts every connection the proxy is forwarding. What the dump experiences is
// exactly what it would see if the server it was reading from went away - a
// connection dying partway through a read - while the deployment the rest of
// the suite shares keeps running, and no second mongod has to be started,
// waited for, and cleaned up after.
func (s *DumpRestoreSuite) TestDumpFailsWhenServerDies() {
	const (
		dbName   = "foo"
		collName = "bar"
	)

	proxy := s.startKillableProxy(s.SingleNodeHost())
	s.seedSlowDumpData(s.Client().Database(dbName).Collection(collName))

	// The dump reads every document through a $where that sleeps, so it is
	// still running when the connection is cut. Without that it would finish
	// first and there would be nothing to interrupt.
	ranFor, dumpErr := s.dumpWhileCuttingConnection(
		proxy,
		"--out", s.T().TempDir(),
		"--db", dbName,
		"--collection", collName,
		"--query", `{"$where": "sleep(25); return true;"}`,
	)

	// Without this, a dump that failed before the connection was cut - because
	// the options were wrong, say, or the query was rejected - would satisfy
	// every assertion below.
	s.Require().Greater(
		ranFor,
		dumpHeadStart,
		"the dump was still running when its connection was cut",
	)

	s.Require().NotNil(dumpErr, "mongodump fails when its connection to the server dies mid-dump")
	s.Require().Regexp(
		serverDiedErrors,
		dumpErr.Error(),
		"mongodump reports that it could not read from the server",
	)
}

// serverDiedErrors matches the errors mongodump can surface for a connection
// that died, depending on how far into the read it got and where the failure
// surfaced. It deliberately does not accept "connection refused": that is what
// a dump reports when it never reached the server at all, which is a different
// failure.
//
// "incomplete read of message header" is the driver's own wording for a
// connection that died partway through a wire message, and "connection closed"
// covers the driver's wording for one that died between wire messages. Only
// those prefixes are portable - the wrapped error after them is whatever the
// OS calls a closed connection, "connection reset by peer" on Linux and
// "wsarecv: An existing connection was forcibly closed by the remote host" on
// Windows.
const serverDiedErrors = `(?i)error reading from db|error reading collection|` +
	`connection closed|interrupted|socket was unexpectedly closed|` +
	`incomplete read of message header`

// killableProxy forwards TCP connections to one target host until it is
// killed. Killing it closes the listener and both sides of every connection it
// is forwarding: the client side is what the dump experiences as its server
// dying mid-read, and the server side is what tells the deployment the client
// is gone, so that it stops the work the dump left behind.
type killableProxy struct {
	listener net.Listener
	target   string

	mutex   sync.Mutex
	killed  bool
	conns   []net.Conn
	accepts int
}

func (s *DumpRestoreSuite) startKillableProxy(target string) *killableProxy {
	listener, err := net.Listen("tcp", "localhost:0")
	s.Require().NoError(err, "can listen for the connections the dump will make")

	proxy := &killableProxy{listener: listener, target: target}
	s.T().Cleanup(proxy.kill)

	go func() {
		for {
			conn, err := proxy.listener.Accept()
			if err != nil {
				return
			}
			go proxy.forward(conn)
		}
	}()

	return proxy
}

// host is the host:port to point the dump at, which reaches the target through
// the proxy.
func (p *killableProxy) host() string {
	return p.listener.Addr().String()
}

func (p *killableProxy) forward(clientConn net.Conn) {
	serverConn, err := net.Dial("tcp", p.target)
	if err != nil {
		_ = clientConn.Close()

		return
	}

	p.mutex.Lock()
	// A connection accepted just before kill() can still be dialing the target
	// when kill() drains the conns it knows about. Registering it afterwards
	// would leave a live path through the supposedly dead proxy, so a dial that
	// lands late finds the proxy already killed and closes both sides instead.
	if p.killed {
		p.mutex.Unlock()
		_ = clientConn.Close()
		_ = serverConn.Close()

		return
	}
	p.conns = append(p.conns, clientConn, serverConn)
	p.accepts++
	p.mutex.Unlock()

	go func() {
		_, _ = io.Copy(serverConn, clientConn)
		_ = serverConn.Close()
	}()

	_, _ = io.Copy(clientConn, serverConn)
	_ = clientConn.Close()
}

// accepted returns how many connections the proxy has taken, which tells a
// dump that connected through it apart from one that went around it.
func (p *killableProxy) accepted() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.accepts
}

// kill closes the proxy's listener and both sides of every connection it is
// forwarding. Closing an already-closed listener or connection is not an error
// here, so its return value is thrown away.
func (p *killableProxy) kill() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.killed = true
	_ = p.listener.Close()
	for _, conn := range p.conns {
		_ = conn.Close()
	}
	p.conns = nil
}

func (s *DumpRestoreSuite) seedSlowDumpData(coll *mongo.Collection) {
	docs := make([]any, slowDumpDocCount)
	for i := range docs {
		docs[i] = bson.D{{"x", i}}
	}
	_, err := coll.InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the documents the dump will read")
}

// slowDumpDocCount is high enough that the dump's sleeping $where keeps it
// running for far longer than it takes to cut the connection underneath it.
const slowDumpDocCount = 1000

// dumpWhileCuttingConnection starts a dump reading through the proxy and cuts
// the proxy's connections once the dump is under way. It returns whatever the
// dump reports, along with how long the dump itself ran, which is what tells a
// dump the cut interrupted apart from one that had already failed for some
// other reason.
func (s *DumpRestoreSuite) dumpWhileCuttingConnection(
	proxy *killableProxy,
	dumpArgs ...string,
) (time.Duration, error) {
	// DirectToolArgs keeps whatever auth and TLS configuration the deployment
	// needs and forces a direct connection, so that the dump cannot discover
	// the rest of a replica set and retry its read against a member the proxy
	// does not front - which would turn the cut connection into a dump that
	// simply succeeds.
	opts, err := mongodump.ParseOptions(
		slices.Concat(s.DirectToolArgs(proxy.host()), dumpArgs),
		"",
		"",
	)
	s.Require().NoError(err, "can parse the mongodump options")

	// This is set here rather than passed as --serverSelectionTimeout because
	// the flag conflicts with a URI that carries its own serverSelectionTimeoutMS
	// and makes ParseOptions fail before the test runs. The field is what
	// client creation reads, so setting it after parsing still takes effect.
	opts.ToolOptions.ServerSelectionTimeout = serverSelectionTimeoutSeconds

	dump := &mongodump.MongoDump{
		ToolOptions:   opts.ToolOptions,
		InputOptions:  opts.InputOptions,
		OutputOptions: opts.OutputOptions,
	}
	s.Require().NoError(dump.Init(), "mongodump can connect through the proxy")
	s.Require().Greater(
		proxy.accepted(),
		0,
		"the dump connected through the proxy rather than around it",
	)

	cut := make(chan struct{})
	go func() {
		time.Sleep(dumpHeadStart)
		proxy.kill()
		close(cut)
	}()

	start := time.Now()
	dumpErr := dump.Dump()
	ranFor := time.Since(start)

	<-cut

	return ranFor, dumpErr
}

// serverSelectionTimeoutSeconds keeps the dump from waiting out the driver's
// default. When the connection is cut the driver retries the read, and that
// retry sits in server selection waiting for the proxy to come back before the
// dump reports the failure it already had.
const serverSelectionTimeoutSeconds = 2

// dumpHeadStart is how long the dump is left running before its connection is
// cut. The dump takes tens of seconds to finish, so this only has to be long
// enough for it to have started reading.
const dumpHeadStart = time.Second
