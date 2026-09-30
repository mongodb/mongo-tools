package writeconcern

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type WriteConcernSuite struct {
	sharedsuite.IntegrationSuite
}

func TestWriteConcern(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	suite.Run(t, new(WriteConcernSuite))
}

// replicationSettleTime gives the secondaries a moment to finish applying
// whatever was already in flight when the failpoint went on, so that a write
// made afterwards really is one they will not acknowledge.
const replicationSettleTime = 2 * time.Second

// stopReplicationOn makes the given secondaries stop applying oplog entries, so
// that a write concern asking for more acknowledgements than the remaining nodes
// can give cannot be satisfied. Replication starts again when the test ends.
func (s *WriteConcernSuite) stopReplicationOn(hosts []string) {
	for _, host := range hosts {
		s.setSyncFailPoint(host, "alwaysOn")

		// The context the suite hands out belongs to the running test, so starting
		// replication again afterwards needs one that outlives the test.
		teardownCtx := context.WithoutCancel(s.Context())
		s.T().Cleanup(func() {
			s.startReplicationOn(teardownCtx, host)
		})
	}

	if len(hosts) > 0 {
		time.Sleep(replicationSettleTime)
	}
}

func (s *WriteConcernSuite) startReplicationOn(ctx context.Context, host string) {
	err := s.syncFailPointErr(ctx, host, "off")
	if err == nil {
		return
	}

	// A connection dropped during teardown is the realistic failure here, and it
	// does not mean the command will fail again.
	retryErr := s.syncFailPointErr(ctx, host, "off")
	if retryErr == nil {
		return
	}

	// rsSyncApplyStop is state on a server that every integration package shares,
	// and they run one after another. Leaving it on would make every later test
	// run against secondaries that never apply oplog entries, which surfaces as
	// unrelated timeouts far from the cause, so end the run here instead of
	// reporting this as one test's failure.
	panic(fmt.Sprintf(
		"could not start replication again on %s, which leaves the replica set"+
			" unusable for every later test: %v (on retry: %v)",
		host, err, retryErr,
	))
}

func (s *WriteConcernSuite) setSyncFailPoint(host, mode string) {
	s.Require().NoError(
		s.syncFailPointErr(s.Context(), host, mode),
		"can set rsSyncApplyStop to %#q on %s", mode, host,
	)
}

func (s *WriteConcernSuite) syncFailPointErr(ctx context.Context, host, mode string) error {
	client := s.DirectClient(host)
	defer func() {
		s.Assert().NoError(client.Disconnect(ctx), "can disconnect from %s", host)
	}()

	return client.Database("admin").
		RunCommand(ctx, bson.D{{"configureFailPoint", "rsSyncApplyStop"}, {"mode", mode}}).
		Err()
}
