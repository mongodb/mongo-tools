package evergreen

import (
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type aliasEntry struct {
	Variant string `yaml:"variant"`
	Task    string `yaml:"task"`
}

// Every task that the merge-queue variant depends on must run for PRs, otherwise PRs will not be
// mergeable at all, because the GitHub branch protection ruleset for this repo requires that
// `evergreen/merge-queue` has passed.
//
// This is a subset check, not equality. PRs can run things that the merge queue does not depend on.
func TestPRTasksAreRequiredForMerge(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	c, err := Load()
	require.NoError(t, err, "loading common.yml")

	prYAML, err := c.GitHubPRAliasesYAML()
	require.NoError(t, err, "generating PR aliases")

	// Expanding the tag selectors in common.yml means running the evergreen CLI,
	// which the project never installs -- it relies on Evergreen hosts having it.
	// Rather than fail the unit suite on every variant that lacks it, skip.
	if _, err := exec.LookPath("evergreen"); err != nil {
		t.Skip("this test needs the evergreen CLI on PATH to expand task tags")
	}

	evaluated, err := LoadEvaluated()
	require.NoError(t, err, "evaluating common.yml")

	prTasks := evaluated.tasksMatchingAliases(t, parseAliases(t, prYAML, "github_pr_aliases"))
	missingFromPRs := evaluated.mergeQueueDependencies(t).Difference(prTasks)

	assert.Empty(
		t,
		sortedNames(missingFromPRs),
		"every task the %q variant depends on is selected by github_pr_aliases,"+
			" so add an alias covering it (or drop the dependency)",
		mergeQueueVariant,
	)
}

// parseAliases pulls one alias sequence out of a YAML document. It decodes only the requested key,
// so that it works on all of common.yml and not just on a generated block.
func parseAliases(t *testing.T, doc, key string) []aliasEntry {
	t.Helper()

	var parsed map[string]yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(doc), &parsed), "the %s YAML parses", key)

	node, ok := parsed[key]
	require.True(t, ok, "the YAML contains the %s key", key)

	var entries []aliasEntry
	require.NoError(t, node.Decode(&entries), "the %s value is a list of aliases", key)

	return entries
}

// The no-op task the merge queue gate variant runs.
const mergeQueueWorkaroundTaskName = "merge-queue-workaround"

// variantTask is one scheduled unit of work: a task on a specific variant.
type variantTask struct {
	variant string
	task    string
}

// tasksMatchingAliases is every task the given aliases select. The receiver must come from
// LoadEvaluated, so that each variant's task list is concrete names rather than tag selectors.
func (c *Config) tasksMatchingAliases(
	t *testing.T,
	aliases []aliasEntry,
) mapset.Set[variantTask] {
	t.Helper()

	matched := mapset.NewSet[variantTask]()
	for _, a := range aliases {
		variantRE, err := regexp.Compile(a.Variant)
		require.NoError(t, err, "compiling the variant regex %q", a.Variant)

		taskRE, err := regexp.Compile(a.Task)
		require.NoError(t, err, "compiling the task regex %q", a.Task)

		for _, v := range c.Variants {
			if !variantRE.MatchString(v.Name) {
				continue
			}

			for _, task := range v.Tasks {
				if taskRE.MatchString(task.Name) {
					matched.Add(variantTask{v.Name, task.Name})
				}
			}
		}
	}

	return matched
}

// mergeQueueDependencies is every task the gate task depends on. It does not follow those tasks'
// own dependencies, which is the conservative direction for the caller: a transitively required
// task shows up as one to check rather than being silently passed over.
func (c *Config) mergeQueueDependencies(t *testing.T) mapset.Set[variantTask] {
	t.Helper()

	deps := mapset.NewSet[variantTask]()
	for _, task := range c.variantTasks(t, mergeQueueVariant) {
		if task.Name != mergeQueueWorkaroundTaskName {
			continue
		}

		require.NotEmpty(
			t,
			task.DependsOn,
			"the %q task declares dependencies",
			mergeQueueWorkaroundTaskName,
		)

		for _, dep := range task.DependsOn {
			require.NotEqual(
				t,
				"*",
				dep.Variant,
				`the dependency on %q names a specific variant, because a wildcard`+
					` would silently pull in new variants`,
				dep.Name,
			)

			// Evergreen expands tag selectors but leaves `*` alone.
			if dep.Name == "*" {
				for _, task := range c.variantTasks(t, dep.Variant) {
					deps.Add(variantTask{dep.Variant, task.Name})
				}
				continue
			}

			deps.Add(variantTask{dep.Variant, dep.Name})
		}
	}

	require.NotZero(t, deps.Cardinality(), "the %q variant gates some tasks", mergeQueueVariant)

	return deps
}

func (c *Config) variantTasks(t *testing.T, variant string) []Task {
	t.Helper()

	for _, v := range c.Variants {
		if v.Name == variant {
			return v.Tasks
		}
	}

	require.FailNowf(t, "unknown variant", "common.yml defines a %q variant", variant)

	return nil
}

// sortedNames renders a set of scheduled tasks as sorted "variant/task" strings,
// so that a failure names the offending tasks in a stable order.
func sortedNames(tasks mapset.Set[variantTask]) []string {
	names := make([]string, 0, tasks.Cardinality())
	for _, vt := range tasks.ToSlice() {
		names = append(names, vt.variant+"/"+vt.task)
	}
	slices.Sort(names)

	return names
}

// The github_pr_aliases block in common.yml is generated. This catches both a
// hand-edit and a change to the generator that nobody copied into common.yml.
//
// Comparing the parsed alias sequences rather than searching for the generated
// text matters: a substring search would not notice a hand-written alias
// appended after the generated block.
func TestGitHubPRAliasBlockIsUpToDate(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	c, err := Load()
	require.NoError(t, err, "loading common.yml")

	block, err := c.GitHubPRAliasesYAML()
	require.NoError(t, err, "generating the github_pr_aliases block")

	common, err := os.ReadFile(commonYAMLPath())
	require.NoError(t, err, "reading common.yml")

	assert.Equal(
		t,
		parseAliases(t, block, "github_pr_aliases"),
		parseAliases(t, string(common), "github_pr_aliases"),
		"the github_pr_aliases block in common.yml matches the generator output"+
			" (run `go run evergreen/generator/main.go` and replace the block in"+
			" common.yml with its output)",
	)
}
