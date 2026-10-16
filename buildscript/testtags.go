package buildscript

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/craiggwilson/goke/task"
)

// buildTagRE matches a //go:build directive.
var buildTagRE = regexp.MustCompile(`(?m)^//go:build `)

// testFuncRE matches the top-level test, benchmark, and example functions that `go test` runs.
var testFuncRE = regexp.MustCompile(`(?m)^func (Test|Benchmark|Example)`)

// serverTestRE matches references that only make sense in a test that talks to a real server, plus
// the testtype skip helpers that the build tags replaced. A helper-only _test.go file matching one
// of these must have a build tag, or it will be compiled into every test run.
//
// testutil.GetBareSession covers GetBareSessionProvider and GetBareSessionForURI too, testcmd.Run
// covers RunAgainstTestServer, and testutil.Skip covers the SkipFor*/SkipIf*/SkipUnless* helpers.
var serverTestRE = regexp.MustCompile(
	`testutil\.(GetBareSession|SetUpTimeseries|Skip)|db\.NewSessionProvider|` +
		`testcmd\.(Run|ServerHostPort|SingleHostURI)|` +
		`testtype\.(SkipUnless|RequireMatching|HasTestType)`,
)

// SACheckTestBuildTags fails if a _test.go file that defines tests has no build tag, or if a
// helper-only test file that talks to a server has none. Without a tag such a file compiles into
// every test run: it either runs when it should not, or quietly never runs at all.
func SACheckTestBuildTags(ctx *task.Context) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	var offenders []string

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if buildTagRE.Match(content) {
			return nil
		}
		// A file with test functions always needs a tag. A helper-only file needs one only
		// when it talks to a server.
		if !testFuncRE.Match(content) && !serverTestRE.Match(content) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		offenders = append(offenders, rel)

		return nil
	})
	if err != nil {
		return fmt.Errorf("scanning test files: %w", err)
	}

	if len(offenders) > 0 {
		return fmt.Errorf(
			"these test files define tests, or talk to a server, but have no build tag:\n  %s",
			strings.Join(offenders, "\n  "),
		)
	}

	return nil
}
