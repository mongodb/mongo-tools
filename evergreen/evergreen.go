package evergreen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/mitchellh/go-wordwrap"
)

// Config is the part of the Evergreen project configuration this package reads. It is deliberately
// incomplete: generating the alias blocks only needs variant and task names.
//
// The DependsOn field is only used by this package's tests, which check that every required for the
// merge queue is also run for PRs.
type Config struct {
	Variants []Variant `yaml:"buildvariants"`
}

type Variant struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
	Tasks       []Task `yaml:"tasks"`
}

type Task struct {
	Name string `yaml:"name"`

	// Evergreen pushes a variant's depends_on down onto each of its tasks when it evaluates a
	// configuration, so this is where the merge queue gate's dependencies show up.
	DependsOn []Dependency `yaml:"depends_on"`
}

// Dependency is one entry in a task's depends_on list. Name may be the literal `*`, meaning every
// task on the named variant; Evergreen leaves that unexpanded.
type Dependency struct {
	Name    string `yaml:"name"`
	Variant string `yaml:"variant"`
}

func repoRoot() string {
	_, pkgPath, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(pkgPath), "..")
}

func commonYAMLPath() string {
	return filepath.Join(repoRoot(), "common.yml")
}

// Load parses common.yml as written, with task selectors like `.latest` left alone. The alias
// generator needs them that way, because it reads the version tags in each variant's task list.
func Load() (*Config, error) {
	common, err := os.ReadFile(commonYAMLPath())
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(common, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadEvaluated returns the configuration as Evergreen itself resolves it: tag selectors expanded
// to concrete task names, and the files in common.yml's `include:` list merged in.
//
// Only this package's tests use it. Shelling out to the evergreen CLI is the point: reimplementing
// tag and selector semantics in Go would be a second, subtly different copy of Evergreen's own
// rules, and a test built on a wrong copy would pass while checking the wrong thing. The command is
// local -- it makes no network calls and needs no credentials -- but it does require the evergreen
// CLI on PATH.
func LoadEvaluated() (*Config, error) {
	cmd := exec.Command("evergreen", "evaluate", "--variants", "--file", "common.yml")

	// The paths in common.yml's `include:` list are relative to the project root,
	// and evergreen resolves them against its working directory, which is the
	// package directory when this runs under `go test`.
	cmd.Dir = repoRoot()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running `%s`: %w: %s", cmd, err, stderr.String())
	}

	var config Config
	if err := yaml.Unmarshal(stdout.Bytes(), &config); err != nil {
		return nil, fmt.Errorf("parsing the output of `%s`: %w", cmd, err)
	}

	return &config, nil
}

func (v *Variant) includesLatest() bool {
	for _, t := range v.Tasks {
		if t.Name == ".latest" {
			return true
		}
	}
	return false
}

type alias struct {
	comment string
	variant string
	tasks   string
}

// The variant whose only task is a no-op depending on everything that must pass before a merge. It
// is the single required GitHub status check, so the queue needs to select nothing else: Evergreen
// pulls in the dependency closure.  That makes commit_queue_aliases a constant, so it is written
// out by hand in common.yml rather than generated here.
const mergeQueueVariant = "merge-queue"

func (c *Config) GitHubPRAliasesYAML() (string, error) {
	aliases, err := c.aliases()
	if err != nil {
		return "", err
	}

	return aliasesYAML("github_pr_aliases", aliases)
}

func (c *Config) aliases() ([]alias, error) {
	aliases := []alias{
		{
			comment: "Run all static analysis tasks.",
			variant: `^static$`,
			tasks:   `.*`,
		},
		{
			comment: "Run unit for every platform.",
			variant: `.*`,
			tasks:   `^unit$`,
		},
		{
			comment: "Run push and record this PR run in Papertrail.",
			variant: `.*`,
			tasks:   `^push$`,
		},
		{
			comment: "Report the merge queue gate on the PR too. Without this the" +
				" required status check is never reported on the PR ref and" +
				" GitHub waits for it forever.",
			variant: "^" + mergeQueueVariant + "$",
			tasks:   `.*`,
		},
		{
			comment: "Run golang tests with the race detector enabled.",
			variant: `^rhel88-race$`,
			tasks:   `^(aws-auth|integration|kerberos|unit)`,
		},
		{
			comment: "Run all integration tests on one variant. We pick RHEL 8.8 because" +
				" it's a relatively recent platform that supports a wide range of" +
				" servers.",
			variant: `^rhel88$`,
			tasks:   `^(aws-auth|integration|kerberos|legacy|native-cert-ssl|qa-dump-restore|qa-tests)`,
		},
		{
			comment: "Run the integration tests against a disaggregated-storage cluster.",
			variant: `^ubuntu2204-arm64-dsc$`,
			tasks:   `.*`,
		},
	}

	// This finds the most recent version of the server supported by each variant. Based on that it
	// constructs a set of aliases to run "integration-<version>" for that latest version of each
	// supported variant.
	intTests, err := c.integrationTestAliases()
	if err != nil {
		return nil, err
	}
	aliases = append(aliases, intTests...)

	return aliases, nil
}

// The comments are the reason this attaches a comment map rather than marshaling a plain struct.
// The encoder emits each comment's lines before the node it annotates, including the `#` prefix and
// the indentation, but it does not wrap, so each comment arrives here already wrapped to a width
// that leaves room for the prefix the encoder adds.
func aliasesYAML(key string, aliases []alias) (string, error) {
	header := []string{
		" This set of aliases is generated by the code in the `evergreen` directory.",
		" Do not edit this by hand!",
		" It can be regenerated by running `go run evergreen/generator/main.go`.",
	}

	entries := make([]aliasYAML, 0, len(aliases))
	comments := yaml.CommentMap{
		"$." + key: {yaml.HeadComment(header...)},
	}
	for i, a := range aliases {
		entries = append(entries, aliasYAML{
			Variant: quotedString(a.variant),
			Task:    quotedString(a.tasks),
		})
		if a.comment != "" {
			comments[fmt.Sprintf("$.%s[%d]", key, i)] = []*yaml.Comment{
				yaml.HeadComment(commentLines(a.comment)...),
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(
		&buf,
		yaml.Indent(2),
		yaml.IndentSequence(true),
		yaml.WithComment(comments),
	)
	if err := enc.Encode(map[string][]aliasYAML{key: entries}); err != nil {
		return "", fmt.Errorf("encoding the %s aliases: %w", key, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("closing the encoder for the %s aliases: %w", key, err)
	}

	return buf.String(), nil
}

// A width that leaves room for the `#` prefix and indentation the encoder adds, so that no comment
// line exceeds the line length the linter enforces.
const aliasCommentWidth = 72

// commentLines wraps a comment to the linter's width and prefixes each line with the space that
// separates the encoder's `#` from the comment text.
func commentLines(comment string) []string {
	lines := strings.Split(wordwrap.WrapString(comment, aliasCommentWidth), "\n")
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return lines
}

// aliasYAML is the wire form of an alias. The fields are quoted because the aliases are regular
// expressions, and quoting them keeps the generated block unambiguous.
type aliasYAML struct {
	Variant quotedString `yaml:"variant"`
	Task    quotedString `yaml:"task"`
}

// quotedString marshals as a double-quoted YAML scalar.
type quotedString string

func (q quotedString) MarshalYAML() ([]byte, error) {
	return []byte(strconv.Quote(string(q))), nil
}

func (c *Config) integrationTestAliases() ([]alias, error) {
	variantsByServerVersion := make(map[string][]string)
	for _, v := range c.Variants {
		// This is a special case that is covered already.
		if v.Name == "rhel88-race" {
			continue
		}
		if v.includesLatest() {
			variantsByServerVersion["latest"] = append(variantsByServerVersion["latest"], v.Name)
		} else {
			mostRecent, err := v.mostRecentServerVersion()
			if err != nil {
				return nil, err
			}
			if mostRecent == "" {
				continue
			}
			variantsByServerVersion[mostRecent] = append(
				variantsByServerVersion[mostRecent],
				v.Name,
			)
		}
	}

	var versions []string
	for ver := range variantsByServerVersion {
		versions = append(versions, ver)
	}
	sort.Strings(versions)

	var aliases []alias
	for _, ver := range versions {
		variants := variantsByServerVersion[ver]
		sort.Strings(variants)
		var variantRegex string
		if len(variants) == 1 {
			variantRegex = fmt.Sprintf("^%s$", variants[0])
		} else {
			variantRegex = fmt.Sprintf("^(%s)$", strings.Join(variants, "|"))
		}

		aliases = append(aliases, alias{
			comment: fmt.Sprintf(
				"Run a subset of integration tests against the %s version of"+
					" MongoDB Server on all variants where that is the most"+
					" recent supported version.", ver,
			),
			variant: variantRegex,
			tasks:   fmt.Sprintf("integration-%s", ver),
		})
	}

	return aliases, nil
}

var versionRE = regexp.MustCompile(`\.(\d+)\.(\d+)$`)

func (v *Variant) mostRecentServerVersion() (string, error) {
	var versions [][2]int
	for _, t := range v.Tasks {
		if matches := versionRE.FindStringSubmatch(t.Name); len(matches) != 0 {
			maj, err := strconv.Atoi(matches[1])
			if err != nil {
				return "", err
			}
			min, err := strconv.Atoi(matches[2])
			if err != nil {
				return "", err
			}
			versions = append(versions, [2]int{maj, min})
		}
	}
	if len(versions) == 0 {
		return "", nil
	}

	sort.SliceStable(versions, func(i, j int) bool {
		majI := versions[i][0]
		majJ := versions[j][0]
		minI := versions[i][1]
		minJ := versions[j][1]

		if majI != majJ {
			return majI > majJ
		}
		return minI > minJ
	})

	return fmt.Sprintf("%d.%d", versions[0][0], versions[0][1]), nil
}
