package buildscript

import (
	"bytes"
	"errors"
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

// testFileSuffixRE captures the tag suffix of a _test.go filename. The longer integration_sharded
// alternative must be listed so that it is not mistaken for a bare sharded suffix.
var testFileSuffixRE = regexp.MustCompile(
	`_(unit|integration_sharded|integration|sharded|kerberos|awsauth|helpers)_test\.go$`,
)

// namingExemptDirs are directories whose _test.go files are exempt from the filename-suffix rule.
// common/json is copied from the Go standard library, and the integration/ suites are identified
// by their directory rather than by their filenames.
var namingExemptDirs = []string{
	filepath.Join("common", "json"),
	"integration",
}

// SACheckTestFiles fails if a _test.go file that defines tests, or that talks to a server, has no
// build tag, or if a test file's name does not match the suffix its build tag implies. A file with
// no tag is a shared helper and must be named *_helpers_test.go.
func SACheckTestFiles(ctx *task.Context) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	var missingTag, badName []string

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

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		tag := buildTag(content)

		// A file with test functions always needs a tag. A helper-only file needs one only
		// when it talks to a server.
		if tag == "" && (testFuncRE.Match(content) || serverTestRE.Match(content)) {
			missingTag = append(missingTag, rel)
		}

		if !isExemptFromNaming(rel) {
			if want := expectedTestFileSuffix(tag); actualTestFileSuffix(path) != want {
				badName = append(badName, fmt.Sprintf("%s (want _%s_test.go)", rel, want))
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("scanning test files: %w", err)
	}

	var problems []string
	if len(missingTag) > 0 {
		problems = append(problems, fmt.Sprintf(
			"these test files define tests, or talk to a server, but have no build tag:\n  %s",
			strings.Join(missingTag, "\n  "),
		))
	}
	if len(badName) > 0 {
		problems = append(problems, fmt.Sprintf(
			"these test files have a name that does not match their build tag:\n  %s",
			strings.Join(badName, "\n  "),
		))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n\n"))
	}

	return nil
}

// buildTag returns the expression in the file's first //go:build directive, or "" if there is none.
func buildTag(content []byte) string {
	loc := buildTagRE.FindIndex(content)
	if loc == nil {
		return ""
	}

	rest := content[loc[1]:]
	if nl := bytes.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}

	return strings.TrimSpace(string(rest))
}

// expectedTestFileSuffix maps a build tag expression to the filename suffix it requires. A file
// with no tag is a shared helper and requires the "helpers" suffix. An unrecognized tag panics: the
// set of tags is closed, so an unknown one is a bug in the test tags or in this function, not
// something to skip silently.
func expectedTestFileSuffix(tag string) string {
	switch {
	case tag == "":
		return "helpers"
	case strings.Contains(tag, TagKerberos.String()):
		return "kerberos"
	case strings.Contains(tag, TagAWSAuth.String()):
		return "awsauth"
	case strings.Contains(tag, TagUnit.String()):
		return "unit"
	case strings.Contains(tag, TagIntegration.String()) && strings.Contains(tag, TagSharded.String()):
		return "integration_sharded"
	case strings.Contains(tag, TagIntegration.String()):
		return "integration"
	case strings.Contains(tag, TagSharded.String()):
		return "sharded"
	}

	panic(fmt.Sprintf("unknown test build tag %q; add a suffix mapping for it", tag))
}

// actualTestFileSuffix returns the tag suffix embedded in a _test.go filename, or "" if the name
// carries no recognized suffix.
func actualTestFileSuffix(path string) string {
	m := testFileSuffixRE.FindStringSubmatch(filepath.ToSlash(path))
	if m == nil {
		return ""
	}

	return m[1]
}

func isExemptFromNaming(rel string) bool {
	for _, dir := range namingExemptDirs {
		if rel == dir || strings.HasPrefix(rel, dir+string(filepath.Separator)) {
			return true
		}
	}

	return false
}
