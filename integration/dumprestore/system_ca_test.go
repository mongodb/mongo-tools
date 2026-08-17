package dumprestore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
)

// TestDumpTrustingTheSystemCA checks that a tool given --ssl but no --sslCAFile
// verifies the server against the platform's own trust store, rather than
// against a CA file of its own or nothing at all.
//
// The check is run as a subprocess for each case because Go caches the system
// certificate pool for the life of a process, so a second run in the same
// process would reuse the first run's set of roots.
func (s *DumpRestoreSuite) TestDumpTrustingTheSystemCA() {
	testtype.SkipUnlessTestType(s.T(), testtype.SSLTestType)

	if runtime.GOOS != "linux" {
		// On darwin and Windows, Go verifies with the platform's own verifier
		// rather than by loading root certificates out of files, so there is no
		// trust store this test can point somewhere.
		s.T().Skipf("SSL_CERT_FILE does not control which roots Go trusts on %s", runtime.GOOS)
	}
	if uri := os.Getenv("TOOLS_TESTING_MONGOD"); uri != "" {
		// The connection arguments below are built by hand rather than with
		// testopts.GetBareArgs, which would add the --sslCAFile this test exists to
		// leave out, so they cannot honor an overridden deployment.
		s.T().Skipf("this test addresses the server directly, not through %#q", uri)
	}

	// The collection gives the successful run something to dump.
	s.insertNamespacedDocs(s.database("system_ca").Collection("bar"))

	s.Run("without the server's CA in the trust store", func() {
		// An empty file rather than no file at all: Go reads its built-in list of
		// certificate files when SSL_CERT_FILE is unset, which would leave the
		// outcome up to what the host trusts.
		noRoots := filepath.Join(s.T().TempDir(), "no-roots.pem")
		s.Require().NoError(os.WriteFile(noRoots, nil, 0o600), "can write an empty CA file")

		stderr, err := s.runMongodumpTrustingTheSystemCA(noRoots)
		s.Require().ErrorAs(
			err,
			new(*exec.ExitError),
			"mongodump fails when the system trust store does not cover the server",
		)
		s.Require().Contains(
			stderr,
			"certificate signed by unknown authority",
			"mongodump fails because it cannot verify the server's certificate",
		)
	})

	s.Run("with the server's CA in the trust store", func() {
		stderr, err := s.runMongodumpTrustingTheSystemCA(testopts.GetSSLOptions().SSLCAFile)
		s.Require().NoError(
			err,
			"mongodump succeeds when the system trust store covers the server (stderr: %s)",
			stderr,
		)
	})
}

// runMongodumpTrustingTheSystemCA runs mongodump with --ssl but no --sslCAFile,
// so that it falls back to the platform trust store. It returns the standard
// error of the run along with its exit status.
//
// SSL_CERT_FILE names the only certificate file Go will trust and SSL_CERT_DIR an
// empty directory, so that what the host itself trusts cannot decide the outcome.
func (s *DumpRestoreSuite) runMongodumpTrustingTheSystemCA(caFile string) (string, error) {
	sslOpts := testopts.GetSSLOptions()

	args := []string{"run", filepath.Join("..", "..", "mongodump", "main")}
	args = append(args, "--ssl", "--sslPEMKeyFile", sslOpts.SSLPEMKeyFile)
	args = append(args, testopts.GetAuthArgs()...)
	args = append(args,
		"--host", "localhost",
		"--port", testopts.DefaultTestPort,
		"--out", s.T().TempDir(),
	)

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(),
		"SSL_CERT_FILE="+caFile,
		"SSL_CERT_DIR="+s.T().TempDir(),
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stderr.String(), err
}
