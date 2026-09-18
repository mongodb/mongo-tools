// Package testcmd holds helpers for tests that run a built tool as a
// subprocess and read what it printed. It lives under internal/ because it is
// test scaffolding for this module rather than something other projects build
// against.
package testcmd

import (
	"bytes"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/stretchr/testify/require"
)

// Run runs the binary with the given arguments and returns its standard output,
// its standard error, and the error from running it.
func Run(t *testing.T, binary string, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

// RunAgainstTestServer is Run with the arguments that point the tool at the
// test deployment prepended.
func RunAgainstTestServer(
	t *testing.T,
	binary string,
	args ...string,
) (string, string, error) {
	t.Helper()

	return Run(t, binary, append(testopts.GetBareArgs(), args...)...)
}

// Rows splits captured output into its non-empty lines, trimmed. The tools that
// print a table pad their columns, so a caller comparing rows needs the padding
// gone and the blank lines dropped.
func Rows(output string) []string {
	var rows []string
	for line := range strings.SplitSeq(output, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			rows = append(rows, trimmed)
		}
	}

	return rows
}

// RequireExitFailure requires that err is the error of a process that exited
// nonzero, rather than a failure to start it at all.
func RequireExitFailure(t *testing.T, err error, toolName string) {
	t.Helper()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "%s exits nonzero", toolName)
}

// ServerHostPort returns the "host:port" of the deployment the tests run
// against, for the tools that take a --host rather than a connection string.
func ServerHostPort(t *testing.T) string {
	t.Helper()

	uri := os.Getenv("TOOLS_TESTING_MONGOD")
	if uri == "" {
		return "localhost:" + testopts.DefaultTestPort
	}

	parsed, err := url.Parse(uri)
	require.NoError(t, err, "TOOLS_TESTING_MONGOD is a URI")
	require.NotEmpty(t, parsed.Host, "TOOLS_TESTING_MONGOD names a host")

	return parsed.Host
}

// UnreachablePort returns a port nothing is listening on, for the tests that
// need a connection to fail rather than succeed against something unexpected.
func UnreachablePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err, "can bind a port to take it out of the ephemeral range")
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err, "the bound address has a port")
	require.NoError(t, listener.Close(), "can release the port again")

	return port
}
