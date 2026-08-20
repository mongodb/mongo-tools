package writeconcern

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongofiles"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

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

	var writeConcernError mongo.WriteConcernError
	if errors.As(err, &writeConcernError) {
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

// TestMongoImportWriteConcern checks that mongoimport succeeds or fails
// according to whether the replica set can satisfy the write concern it was
// given, which is what jstests/import/import_write_concern.js checked.
func (s *WriteConcernSuite) TestMongoImportWriteConcern() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	const collName = "bar"

	dbName := s.DBName()
	importFile := s.writeImportFile()

	s.runWriteConcernCases(dbName, func(writeConcern string) error {
		return s.importWithWriteConcern(dbName, collName, importFile, writeConcern)
	})
}

// TestMongoFilesWriteConcern is TestMongoImportWriteConcern for mongofiles,
// covering jstests/files/mongofiles_write_concern.js.
func (s *WriteConcernSuite) TestMongoFilesWriteConcern() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	dbName := s.DBName()
	putFile := s.writePutFile()

	s.runWriteConcernCases(dbName, func(writeConcern string) error {
		return s.putWithWriteConcern(dbName, putFile, writeConcern)
	})
}

// TestMongoImportWaitsForSlowReplication checks that a write concern with no
// timeout makes mongoimport wait for the secondaries rather than giving up. This
// was the one case in wc_framework.js that tested waiting rather than failing, so
// it is the only one that has to run the tool concurrently with the failpoint.
func (s *WriteConcernSuite) TestMongoImportWaitsForSlowReplication() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	const collName = "bar"

	dbName := s.DBName()
	importFile := s.writeImportFile()

	s.assertWaitsForReplication("mongoimport", func() func() error {
		mi := s.newImport(dbName, collName, importFile, noTimeoutWriteConcern)
		s.T().Cleanup(mi.Close)

		return func() error {
			_, _, err := mi.ImportDocuments()

			return err
		}
	})
}

// TestMongoFilesWaitsForSlowReplication is the same for mongofiles, which
// wc_framework.js also ran this case against. It writes through GridFS rather
// than an insert, so the waiting is not the same code as the import's.
func (s *WriteConcernSuite) TestMongoFilesWaitsForSlowReplication() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	dbName := s.DBName()
	putFile := s.writePutFile()

	s.assertWaitsForReplication("mongofiles", func() func() error {
		mf := s.newPut(dbName, putFile, noTimeoutWriteConcern)
		s.T().Cleanup(mf.Close)

		return func() error {
			_, err := mf.Run(false)

			return err
		}
	})
}

// noTimeoutWriteConcern asks for every node in the set with no wtimeout, so the
// tool has to wait however long replication takes rather than giving up.
const noTimeoutWriteConcern = "{w:3}"

// assertWaitsForReplication stops every secondary from replicating, starts the
// tool, and checks that it neither returns nor fails until replication resumes.
// prepare builds the tool and returns the func that runs it: building it makes
// assertions, so it has to happen here on the test's goroutine rather than in the
// goroutine the tool runs on.
func (s *WriteConcernSuite) assertWaitsForReplication(tool string, prepare func() func() error) {
	secondaries := s.SecondaryHosts()
	s.stopReplicationOn(secondaries)

	run := prepare()

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
		s.startReplicationOn(s.Context(), host)
	}

	select {
	case err := <-finished:
		s.Assert().NoError(err, tool+" succeeds once the secondaries catch up")
	case <-time.After(catchUpTimeout):
		s.Require().Fail(tool + " finishes once the secondaries replicate again")
	}
}

const (
	// mustKeepWaitingFor is how long the tool has to still be running for, to show
	// it is waiting rather than returning early.
	mustKeepWaitingFor = 2 * time.Second

	// catchUpTimeout bounds how long the tool may take to finish once the
	// secondaries replicate again. It only has to exceed what a healthy set needs,
	// which is well under a second.
	catchUpTimeout = 30 * time.Second
)

// runWriteConcernCases runs every row of the matrix against one tool. The target
// database is dropped before replication is stopped, matching the order
// wc_framework.js used: a drop needs the whole set, so it cannot happen while
// the secondaries are held back.
func (s *WriteConcernSuite) runWriteConcernCases(
	dbName string,
	run func(writeConcern string) error,
) {
	// The cases hold back up to two secondaries, so a smaller set would make
	// SecondaryHosts()[:n] panic. Only an environment variable gates this test, so
	// say what is wrong rather than panicking on a set that is too small.
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

			s.stopReplicationOn(s.SecondaryHosts()[:c.stoppedSecondaries])

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

// writeConcernCase is one row of the matrix that jstests/libs/wc_framework.js
// ran: some number of secondaries stopped from replicating, a write concern, and
// whether the tool is expected to fail.
type writeConcernCase struct {
	stoppedSecondaries int
	writeConcern       string
	wantFailure        bool
}

func (c writeConcernCase) name() string {
	concern := c.writeConcern
	if concern == "" {
		concern = "no write concern"
	}

	return fmt.Sprintf("%s with %d stopped secondaries", concern, c.stoppedSecondaries)
}

// writeConcernCases is the matrix from wc_framework.js. The write concerns that
// ask for more acknowledgements than there are nodes still replicating are the
// ones expected to fail.
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

func (s *WriteConcernSuite) importWithWriteConcern(
	dbName, collName, path, writeConcern string,
) error {
	mi := s.newImport(dbName, collName, path, writeConcern)
	defer mi.Close()

	imported, failed, err := mi.ImportDocuments()
	if err != nil {
		return err
	}

	// A write concern the set can satisfy has to leave the documents behind. Without
	// this an import of nothing at all would pass every succeeding case.
	s.Assert().Zero(failed, "no document fails to import")
	s.Assert().EqualValues(wantImportedDocs, imported, "every document is imported")

	return nil
}

// newImport builds the tool separately from running it, so that a test running
// the import on another goroutine can keep the steps that make assertions on the
// test's own goroutine, where testify is safe to call.
func (s *WriteConcernSuite) newImport(
	dbName, collName, path, writeConcern string,
) *mongoimport.MongoImport {
	args := append(s.DirectToolArgs(s.PrimaryHost()),
		"--db", dbName,
		"--collection", collName,
		"--file", path,
	)
	args = appendWriteConcern(args, writeConcern)

	opts, err := mongoimport.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongoimport options")

	mi, err := mongoimport.New(opts)
	s.Require().NoError(err, "mongoimport can connect")

	return mi
}

func (s *WriteConcernSuite) putWithWriteConcern(dbName, path, writeConcern string) error {
	mf := s.newPut(dbName, path, writeConcern)
	defer mf.Close()

	if _, err := mf.Run(false); err != nil {
		return err
	}

	// As with the import, a write concern the set can satisfy has to leave the file
	// behind, so that storing nothing cannot pass as a success. mongofiles stores
	// the path it was given as the file's name, rather than just the base name.
	stored, err := s.Client().Database(dbName).
		Collection(gridFSFilesCollection).
		CountDocuments(s.Context(), bson.D{{"filename", path}})
	s.Assert().NoError(err, "can count the files mongofiles stored")
	s.Assert().EqualValues(1, stored, "the file mongofiles put is stored")

	return nil
}

const gridFSFilesCollection = "fs.files"

// newPut is newImport for mongofiles, split from running it for the same reason.
func (s *WriteConcernSuite) newPut(dbName, path, writeConcern string) *mongofiles.MongoFiles {
	args := append(s.DirectToolArgs(s.PrimaryHost()), "--db", dbName)
	args = appendWriteConcern(args, writeConcern)
	args = append(args, "put", path)

	opts, err := mongofiles.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongofiles options")

	mf, err := mongofiles.New(opts)
	s.Require().NoError(err, "mongofiles can connect")

	return mf
}

func appendWriteConcern(args []string, writeConcern string) []string {
	if writeConcern == "" {
		return args
	}

	return append(args, "--writeConcern="+writeConcern)
}

// writeImportFile writes the documents that the import cases load. The contents
// do not matter beyond being a valid import, so this is the fixture
// import_write_concern.js built with an export.
func (s *WriteConcernSuite) writeImportFile() string {
	lines := make([]byte, 0, wantImportedDocs*24)
	for i := range wantImportedDocs {
		lines = append(lines, fmt.Sprintf("{\"_id\":%d,\"x\":%d}\n", i, i*i)...)
	}

	path := filepath.Join(s.T().TempDir(), "wc.json")
	s.Require().NoError(os.WriteFile(path, lines, 0600), "can write the import file")

	return path
}

const wantImportedDocs = 101

func (s *WriteConcernSuite) writePutFile() string {
	path := filepath.Join(s.T().TempDir(), "wc.txt")
	s.Require().NoError(
		os.WriteFile(path, []byte("mongofiles write concern test data\n"), 0600),
		"can write the file that mongofiles puts",
	)

	return path
}
