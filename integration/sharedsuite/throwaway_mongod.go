package sharedsuite

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/release/platform"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ThrowawayMongod is a mongod the test owns, so it can be killed, left
// uninitiated, or otherwise mistreated without disturbing the deployment the rest
// of the suite shares.
type ThrowawayMongod struct {
	Process *os.Process
	Host    string
	Client  *mongo.Client
	Stderr  *LockedBuffer
}

// LockedBuffer collects a subprocess's output for a failure message. os/exec fills
// it from a goroutine of its own, so reading it while the process is still running
// needs the lock.
type LockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *LockedBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.buffer.Write(p)
}

func (b *LockedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.buffer.String()
}

// StartThrowawayMongod starts a mongod of the suite's own with the given extra
// arguments, and waits for it to answer. It is killed when the test ends.
func (s *IntegrationSuite) StartThrowawayMongod(extraArgs ...string) *ThrowawayMongod {
	binary := s.mongodBinary()
	port := s.FreePort()

	// No auth and no TLS, whatever the deployment under test uses: nothing else
	// connects to this server, and the caller builds its own arguments for it.
	args := []string{
		"--port", port,
		"--dbpath", s.T().TempDir(),
		"--bind_ip", "localhost",
	}
	cmd := exec.CommandContext(s.Context(), binary, append(args, extraArgs...)...)

	// Kept so that a mongod which refuses to start can say why, instead of the test
	// only reporting that it never came up.
	stderr := &LockedBuffer{}
	cmd.Stderr = stderr

	s.Require().NoError(cmd.Start(), "can start a mongod of our own from %s", binary)

	mongod := &ThrowawayMongod{
		Process: cmd.Process,
		Host:    net.JoinHostPort("localhost", port),
		Stderr:  stderr,
	}
	s.T().Cleanup(func() {
		// A test that kills this process itself has already done so; this is for
		// every other path.
		_ = mongod.Process.Kill()
		_ = cmd.Wait()
	})

	mongod.Client = s.waitForMongod(mongod)

	return mongod
}

// mongodBinary returns the path to a mongod the suite can start. CI downloads one
// alongside the built tools and nothing tells the test where it is, so the default
// is a path relative to the directory go test runs in, which is the package
// directory of the test that called this, not of this file. That is two levels
// down from the repo root for every caller under integration/, and a caller at
// another depth has to set the environment variable instead.
func (s *IntegrationSuite) mongodBinary() string {
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

// FreePort returns a port nothing is listening on, by taking one from the operating
// system and handing it straight back. Something else could claim it in between, in
// which case mongod fails to bind and the test reports that it never came up.
func (s *IntegrationSuite) FreePort() string {
	listener, err := net.Listen("tcp", "localhost:0")
	s.Require().NoError(err, "can find a free port to start a mongod on")
	defer func() {
		s.Require().NoError(listener.Close(), "can release the port again")
	}()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	s.Require().NoError(err, "can read the port back out of the listener's address")

	return port
}

// waitForMongod returns a client for a mongod that is answering. It pings rather
// than selecting a writable server, so that it also comes back for a replica set
// member that has no primary.
func (s *IntegrationSuite) waitForMongod(mongod *ThrowawayMongod) *mongo.Client {
	client := s.throwawayClient(mongod.Host)
	s.Require().Eventually(
		func() bool { return client.Ping(s.Context(), nil) == nil },
		mongodStartupTimeout,
		mongodPollInterval,
		"the mongod started on %s comes up; its stderr was:\n%s",
		mongod.Host,
		mongod.Stderr,
	)
	s.T().Cleanup(func() {
		// The server this client is connected to may be dead by the time the test
		// ends, so a failure to disconnect from it is not a failure of the test.
		_ = client.Disconnect(s.Context())
	})

	return client
}

const (
	mongodStartupTimeout = 30 * time.Second
	mongodPollInterval   = 100 * time.Millisecond
)

// throwawayClient connects to a server that has neither auth nor TLS, whatever the
// deployment the suite otherwise talks to requires. It connects directly, so it
// does not need the server to be part of a working replica set.
func (s *IntegrationSuite) throwawayClient(host string) *mongo.Client {
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
