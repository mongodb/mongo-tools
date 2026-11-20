package sharedsuite

import (
	"context"
	"fmt"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// replicationSettleTime gives the secondaries a moment to finish applying
// whatever was already in flight when the failpoint went on, so that a write
// made afterwards really is one they will not acknowledge.
const replicationSettleTime = 2 * time.Second

// replicationRetryDelay is how long StartReplicationOn waits before retrying a
// failed failpoint command, giving a node that just dropped its connection a
// moment to come back.
const replicationRetryDelay = 500 * time.Millisecond

// failPointCommandTimeout puts a deadline on every configureFailPoint command.
// The clients the suite builds set no operation timeout of their own, and the
// teardown context carries no deadline, so without this a node that accepts
// connections but never responds hangs the run with the failpoint still on.
const failPointCommandTimeout = 10 * time.Second

// StopReplicationOn makes the given secondaries stop applying oplog entries, so
// that a write concern asking for more acknowledgements than the remaining nodes
// can give cannot be satisfied. Replication starts again when the test ends.
func (s *IntegrationSuite) StopReplicationOn(hosts []string) {
	for _, host := range hosts {
		// The cleanup is registered before the failpoint is set, so that a set
		// command which fails after the server has already applied the failpoint
		// still leaves a cleanup behind. Turning the failpoint off when it was
		// never turned on is a no-op, so registering unconditionally is safe.
		//
		// The context the suite hands out belongs to the running test, so starting
		// replication again afterwards needs one that outlives the test.
		teardownCtx := context.WithoutCancel(s.Context())
		s.T().Cleanup(func() {
			s.StartReplicationOn(teardownCtx, host)
		})

		s.setSyncFailPoint(host, "alwaysOn")
	}

	if len(hosts) > 0 {
		time.Sleep(replicationSettleTime)
	}
}

func (s *IntegrationSuite) setSyncFailPoint(host, mode string) {
	s.Require().NoError(
		s.syncFailPointErr(s.Context(), host, mode),
		"can set rsSyncApplyStop to %#q on %s", mode, host,
	)
}

// StartReplicationOn makes the given node start applying oplog entries again.
func (s *IntegrationSuite) StartReplicationOn(ctx context.Context, host string) {
	err := s.syncFailPointErr(ctx, host, "off")
	if err == nil {
		return
	}

	// A connection dropped during teardown is the realistic failure here, and it
	// does not mean the command will fail again. The retry waits a moment first
	// rather than repeating the attempt against the same condition immediately.
	time.Sleep(replicationRetryDelay)

	retryErr := s.syncFailPointErr(ctx, host, "off")
	if retryErr == nil {
		return
	}

	// rsSyncApplyStop is state on a server that every test package shares, and
	// they run one after another. Leaving it on would make every later test run
	// against secondaries that never apply oplog entries, which surfaces as
	// unrelated timeouts far from the cause, so end the run here instead of
	// reporting this as one test's failure.
	panic(fmt.Sprintf(
		"could not start replication again on %s, which leaves the replica set"+
			" unusable for every later test: %v (on retry: %v)",
		host, err, retryErr,
	))
}

func (s *IntegrationSuite) syncFailPointErr(ctx context.Context, host, mode string) error {
	// The client is built here rather than through DirectClient, which FailNow's
	// on a connection error: this also runs from teardown, where the error has to
	// return so StartReplicationOn can retry it and then panic, not abort through
	// the suite's assertions.
	sessionProvider, err := db.NewSessionProvider(*s.directToolOptions(host))
	if err != nil {
		return fmt.Errorf("can connect directly to %s: %w", host, err)
	}

	client, err := sessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("can get a session for %s: %w", host, err)
	}
	defer func() {
		s.Assert().NoError(client.Disconnect(ctx), "can disconnect from %s", host)
	}()

	// The teardown context has no deadline of its own, so the command needs one.
	cmdCtx, cancel := context.WithTimeout(ctx, failPointCommandTimeout)
	defer cancel()

	return client.Database("admin").
		RunCommand(cmdCtx, bson.D{{"configureFailPoint", "rsSyncApplyStop"}, {"mode", mode}}).
		Err()
}
