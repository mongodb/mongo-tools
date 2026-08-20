package exportimport

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongoimport"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestImportFailsWithoutAPrimary checks that mongoimport reports a failure when
// there is no primary to write to, rather than exiting as if it had succeeded.
// That was TOOLS-690, and it is what no_primary_error_code.js and
// all_primaries_down_error_code.js were for.
//
// Coverage note: both of those ran against a sharded cluster and so exercised
// mongos's wording of the failure — "not master", "unable to target", "could not
// contact primary for replica set" — by stepping every shard's primary down. This
// reaches the same tool-level behavior by other means: a replica set member that
// was never initiated has no primary, and a port with nothing on it has no server
// at all. The mongos-specific messages are no longer covered; what is covered is
// that the tool fails rather than claiming success.
//
// The servers here are ones this test starts, so nothing it does can leave the
// deployment the rest of the suite shares without a primary.
func (s *ExportImportSuite) TestImportFailsWithoutAPrimary() {
	testtype.SkipUnlessTestType(s.T(), testtype.IntegrationTestType)

	importFile := s.writeBasicImportFile()

	s.Run("a replica set member with no primary", func() {
		// Started with --replSet but never initiated, so the member belongs to a set
		// that has no primary and never elects one.
		mongod := s.StartThrowawayMongod("--replSet", "importHasNoPrimary")

		err := s.importAgainstHost(mongod.Host, importFile)
		s.Require().NotNil(err, "mongoimport fails when the set has no primary")
		s.Assert().Regexp(
			noPrimaryErrors,
			err.Error(),
			"mongoimport reports that it could not find a primary",
		)
	})

	s.Run("a port with no server on it", func() {
		host := net.JoinHostPort("localhost", s.FreePort())

		err := s.importAgainstHost(host, importFile)
		s.Require().NotNil(err, "mongoimport fails when nothing is listening")
		s.Assert().Regexp(
			noPrimaryErrors,
			err.Error(),
			"mongoimport reports that it could not reach a server",
		)
	})
}

// noPrimaryErrors matches how the driver reports having no server it can write to.
// Which one surfaces depends on whether the server answered at all, so both the
// server-selection wording and a refused connection are accepted.
const noPrimaryErrors = `(?i)server selection|no primary|not (primary|master)|` +
	`connection refused|could not connect|unable to target`

// importAgainstHost runs an import directly against one host with a short server
// selection timeout, so a host that can never accept the write fails instead of
// waiting out the default.
func (s *ExportImportSuite) importAgainstHost(host, path string) error {
	args := []string{
		"--host", host,
		"--serverSelectionTimeout", strconv.Itoa(noPrimarySelectionTimeoutSeconds),
		"--db", s.DBName(),
		"--collection", "noPrimaryErrorCode",
		"--file", path,
	}

	opts, err := mongoimport.ParseOptions(args, "", "")
	s.Require().NoError(err, "can parse the mongoimport options")

	mi, err := mongoimport.New(opts)
	if err != nil {
		// Nothing is listening on the host in one of these cases, so the tool can
		// fail this early rather than when it comes to write.
		return err
	}
	defer mi.Close()

	_, _, err = mi.ImportDocuments()

	return err
}

// noPrimarySelectionTimeoutSeconds is in seconds, which is what the flag takes.
const noPrimarySelectionTimeoutSeconds = 2

func (s *ExportImportSuite) writeBasicImportFile() string {
	docs, err := bson.MarshalExtJSON(bson.D{{"a", 1}, {"b", 2}}, false, false)
	s.Require().NoError(err, "can marshal the document to import")

	path := filepath.Join(s.T().TempDir(), "basic.json")
	s.Require().NoError(os.WriteFile(path, append(docs, '\n'), 0600), "can write the import file")

	return path
}
