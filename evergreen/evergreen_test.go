package evergreen

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type aliasEntry struct {
	Variant string `yaml:"variant"`
	Task    string `yaml:"task"`
}

func parseAliases(t *testing.T, doc, key string) []aliasEntry {
	t.Helper()

	var parsed map[string][]aliasEntry
	require.NoError(t, yaml.Unmarshal([]byte(doc), &parsed), "generated %s YAML parses", key)

	entries, ok := parsed[key]
	require.True(t, ok, "generated YAML contains the %s key", key)

	return entries
}

// A merge queue run must never gate on a task that PR testing did not already
// run. Otherwise a PR can pass every check, get enqueued, and then fail in the
// queue for a reason no reviewer could have seen.
func TestMergeQueueAliasesAreSubsetOfPRAliases(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	c, err := Load()
	require.NoError(t, err, "loading common.yml")

	prYAML, err := c.GitHubPRAliasesYAML()
	require.NoError(t, err, "generating PR aliases")

	mqYAML, err := c.MergeQueueAliasesYAML()
	require.NoError(t, err, "generating merge queue aliases")

	prAliases := parseAliases(t, prYAML, "github_pr_aliases")
	mqAliases := parseAliases(t, mqYAML, "commit_queue_aliases")

	require.NotEmpty(t, mqAliases, "merge queue selects at least one alias")

	for _, a := range mqAliases {
		assert.Contains(
			t,
			prAliases,
			a,
			"merge queue alias (variant %q, task %q) is also a PR alias",
			a.Variant,
			a.Task,
		)
	}
}

// Tasks with effects outside Evergreen must not run in the queue, which tests
// a commit that has not landed yet. `push` uploads release artifacts to S3 and
// the linux repos, so it is the one alias PR testing runs that the queue skips.
func TestPushIsTheOnlyAliasExcludedFromMergeQueue(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	c, err := Load()
	require.NoError(t, err, "loading common.yml")

	prYAML, err := c.GitHubPRAliasesYAML()
	require.NoError(t, err, "generating PR aliases")

	mqYAML, err := c.MergeQueueAliasesYAML()
	require.NoError(t, err, "generating merge queue aliases")

	mqAliases := parseAliases(t, mqYAML, "commit_queue_aliases")

	var excluded []aliasEntry
	for _, a := range parseAliases(t, prYAML, "github_pr_aliases") {
		if !slices.Contains(mqAliases, a) {
			excluded = append(excluded, a)
		}
	}

	assert.Equal(
		t,
		[]aliasEntry{{Variant: ".*", Task: "^push$"}},
		excluded,
		"push is the only PR alias the merge queue excludes",
	)
}

// The alias blocks in common.yml are generated. This catches both a hand-edit
// and a change to the generator that nobody copied into common.yml.
//
// Comparing the parsed alias sequences rather than searching for the generated
// text matters: a substring search would not notice a hand-written alias
// appended after the generated block.
func TestCommonYAMLAliasBlocksAreUpToDate(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	c, err := Load()
	require.NoError(t, err, "loading common.yml")

	_, pkgPath, _, _ := runtime.Caller(0)
	commonPath := filepath.Join(filepath.Dir(pkgPath), "..", "common.yml")
	common, err := os.ReadFile(commonPath)
	require.NoError(t, err, "reading common.yml")

	var blocks struct {
		PR    []aliasEntry `yaml:"github_pr_aliases"`
		Queue []aliasEntry `yaml:"commit_queue_aliases"`
	}
	require.NoError(t, yaml.Unmarshal(common, &blocks), "parsing common.yml")

	onDisk := map[string][]aliasEntry{
		"github_pr_aliases":    blocks.PR,
		"commit_queue_aliases": blocks.Queue,
	}

	for key, gen := range map[string]func() (string, error){
		"github_pr_aliases":    c.GitHubPRAliasesYAML,
		"commit_queue_aliases": c.MergeQueueAliasesYAML,
	} {
		t.Run(key, func(t *testing.T) {
			block, err := gen()
			require.NoError(t, err, "generating the %s block", key)

			assert.Equal(
				t,
				parseAliases(t, block, key),
				onDisk[key],
				"the %s block in common.yml matches the generator output"+
					" (run `go run evergreen/generator/main.go` and replace the"+
					" block in common.yml with its output)",
				key,
			)
		})
	}
}
