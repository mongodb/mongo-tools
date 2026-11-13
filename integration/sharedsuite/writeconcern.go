package sharedsuite

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// RunWriteConcernCases runs every row of the write-concern matrix against one
// tool. The target database is dropped before replication is stopped, because a
// drop needs the whole set and so cannot happen while the secondaries are held
// back.
func (s *IntegrationSuite) RunWriteConcernCases(
	dbName string,
	run func(writeConcern string) error,
) {
	// The cases hold back up to two secondaries, so a smaller set would make
	// SecondaryHosts()[:n] panic. Only the environment gates these tests, so say
	// what is wrong rather than panicking on a set that is too small.
	s.Require().GreaterOrEqual(
		len(s.SecondaryHosts()), 2,
		"the write concern cases need a replica set with at least two secondaries",
	)

	for _, c := range writeConcernCases() {
		s.Run(c.name(), func() {
			s.Require().NoError(
				s.Client().Database(dbName).Drop(s.Context()),
				"can drop the target database before the case runs",
			)

			s.StopReplicationOn(s.SecondaryHosts()[:c.stoppedSecondaries])

			err := run(c.writeConcern)
			if !c.wantFailure {
				s.Assert().NoError(err, "the replica set can satisfy %s", c.name())

				return
			}

			if s.Assert().NotNil(err, "the replica set cannot satisfy %s", c.name()) {
				s.Assert().True(
					isUnsatisfiedWriteConcern(err),
					"the failure is the write concern going unsatisfied, not something"+
						" else that timed out: %v", err,
				)
			}
		})
	}
}

// writeConcernCase is one row of the matrix: some number of secondaries stopped
// from replicating, a write concern, and whether the tool is expected to fail.
type writeConcernCase struct {
	stoppedSecondaries int
	writeConcern       string
	wantFailure        bool
}

// writeConcernCases is the matrix these tests run. The write concerns that ask
// for more acknowledgements than there are nodes still replicating are the ones
// expected to fail.
func writeConcernCases() []writeConcernCase {
	return []writeConcernCase{
		{0, "", false},
		{0, "majority", false},
		{0, "{w:1,wtimeout:10000}", false},
		{0, "{w:2,wtimeout:10000}", false},
		{1, "{w:1,wtimeout:10000}", false},
		{1, "{w:2,wtimeout:10000}", false},
		{1, "majority", false},
		{1, "{w:3,wtimeout:2000}", true},
		{2, `{w:"majority",wtimeout:2000}`, true},
		{2, "{w:2,wtimeout:10000}", true},
		{2, "{w:1,wtimeout:10000}", false},
	}
}

func (c writeConcernCase) name() string {
	concern := c.writeConcern
	if concern == "" {
		concern = "no write concern"
	}

	return fmt.Sprintf("%s with %d stopped secondaries", concern, c.stoppedSecondaries)
}

// isUnsatisfiedWriteConcern reports whether err is the write concern going
// unsatisfied, rather than some other failure that merely mentions a timeout. A
// broader match would let a case pass on a slow host even if the failpoint that
// holds the secondaries back had done nothing at all.
func isUnsatisfiedWriteConcern(err error) bool {
	var writeException mongo.WriteException
	if errors.As(err, &writeException) && writeException.WriteConcernError != nil {
		return true
	}

	var bulkException mongo.BulkWriteException
	if errors.As(err, &bulkException) && bulkException.WriteConcernError != nil {
		return true
	}

	if _, ok := errors.AsType[mongo.WriteConcernError](err); ok {
		return true
	}

	// Neither tool sends wtimeout to the server: both turn it into a deadline on
	// their own context, so an unsatisfiable write concern usually arrives as an
	// expired context with nothing in it identifying the write concern. That is
	// indistinguishable from a setup step timing out, which happens before any
	// write and so is not what these cases are testing.
	return unsatisfiedWriteConcern.MatchString(err.Error()) &&
		!writeSetupFailure.MatchString(err.Error())
}

var unsatisfiedWriteConcern = regexp.MustCompile(
	`(?i)(context deadline exceeded|write ?concern|waiting for replication timed out)`,
)

// writeSetupFailure matches the failures the tools report before they write
// anything, from mongofiles.Run's node type lookup, client and ping.
var writeSetupFailure = regexp.MustCompile(
	`(?i)(error determining type of node|error getting client|error connecting to host)`,
)

// AppendWriteConcern appends a --writeConcern flag to a tool's arguments. An
// empty write concern means the tool's default, which is selected by passing no
// flag at all.
func AppendWriteConcern(args []string, writeConcern string) []string {
	if writeConcern == "" {
		return args
	}

	return append(args, "--writeConcern="+writeConcern)
}

// AssertWaitsForReplication stops every secondary from replicating, starts the
// tool, and checks that it neither returns nor fails until replication resumes.
// prepare builds the tool for the given write concern and returns the func that
// runs it: building it makes assertions, so it has to happen here on the test's
// goroutine rather than in the goroutine the tool runs on.
func (s *IntegrationSuite) AssertWaitsForReplication(
	tool string,
	prepare func(writeConcern string) func() error,
) {
	secondaries := s.SecondaryHosts()
	s.StopReplicationOn(secondaries)

	run := prepare(noTimeoutWriteConcern)

	finished := make(chan error, 1)
	go func() {
		finished <- run()
	}()

	select {
	case err := <-finished:
		s.Require().Fail(
			tool+" waits for w:3 with no timeout rather than returning",
			"it returned %v while no secondary was replicating", err,
		)
	case <-time.After(mustKeepWaitingFor):
	}

	for _, host := range secondaries {
		s.StartReplicationOn(s.Context(), host)
	}

	select {
	case err := <-finished:
		s.Assert().NoError(err, tool+" succeeds once the secondaries catch up")
	case <-time.After(catchUpTimeout):
		s.Require().Fail(tool + " finishes once the secondaries replicate again")
	}
}

// noTimeoutWriteConcern asks for every node in the set with no wtimeout, so the
// tool has to wait however long replication takes rather than giving up.
const noTimeoutWriteConcern = "{w:3}"

const (
	// mustKeepWaitingFor is how long the tool has to still be running for, to
	// show it is waiting rather than returning early.
	mustKeepWaitingFor = 2 * time.Second

	// catchUpTimeout bounds how long the tool may take to finish once the
	// secondaries replicate again. It only has to exceed what a healthy set
	// needs, which is well under a second.
	catchUpTimeout = 30 * time.Second
)
