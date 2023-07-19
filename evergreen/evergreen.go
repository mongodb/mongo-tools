package evergreen

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"gopkg.in/yaml.v3"
)

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
}

func Load() (*Config, error) {
	_, pkgPath, _, _ := runtime.Caller(0)
	common, err := ioutil.ReadFile(filepath.Join(filepath.Dir(pkgPath), "..", "common.yml"))
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

func (v *Variant) includesLatest() bool {
	for _, t := range v.Tasks {
		if t.Name == ".latest" {
			return true
		}
	}
	return false
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
			return majI < majJ
		}
		return minI < minJ
	})

	return fmt.Sprintf("%d.%d", versions[0][0], versions[0][1]), nil
}

type alias struct {
	comment string
	variant string
	tasks   string
}

var lineStartRE = regexp.MustCompile(`(?m)^`)

func (c *Config) GitHubPRAliasesYAML() (string, error) {
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
			comment: "Run tests with the race detector enabled.",
			variant: `ubuntu-race`,
			tasks:   `^.*$`,
		},
		{
			comment: "Run all integration tests on one variant. We pick RHEL 8.0 because" +
				" it's a relatively recent platform that supports a wide range of" +
				" servers.",
			variant: `rhel80`,
			tasks:   `^(aws-auth|integration|native-cert-ssl|qa-dump-restore|qa-tests)-.*`,
		},
		{
			comment: "Run srv tests on one variant. We pick RHEL 8.0 because" +
				" it's a relatively recent platform that supports a wide range of" +
				" servers.",
			variant: `rhel80`,
			tasks:   `^srv*`,
		},
		{
			comment: "RHEL 8.0 doesn't run against server 3.4, so we do that with RHEL 7.0.",
			variant: `rhel70`,
			tasks:   `^(aws-auth|integration|native-cert-ssl|qa-dump-restore|qa-tests)-3.4$`,
		},
	}

	// This finds the most recent version of the server supported by each
	// variant. Based on that it constructs a set of aliases to run
	// "integration-<version>" for that latest version of each supported
	// variant.
	intTests, err := c.integrationTestAliases()
	if err != nil {
		return "", err
	}
	aliases = append(aliases, intTests...)

	// We generate the YAML by hand because we want to include the comments.
	yaml := "github_pr_aliases:\n"
	for _, a := range aliases {
		if a.comment != "" {
			wrapped := wordwrap.WrapString(a.comment, 72)
			wrapped = lineStartRE.ReplaceAllString(wrapped, "  # ")
			yaml += fmt.Sprintf("%s\n", wrapped)
		}
		yaml += fmt.Sprintf(`  - variant: "%s"`, a.variant)
		yaml += "\n"
		yaml += fmt.Sprintf(`    task: "%s"`, a.tasks)
		yaml += "\n"
	}

	return yaml, nil
}

func (c *Config) integrationTestAliases() ([]alias, error) {
	variantsByServerVersion := make(map[string][]string)
	for _, v := range c.Variants {
		// This is a special case that is covered already.
		if v.Name == "ubuntu-race" {
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
			variantsByServerVersion[mostRecent] = append(variantsByServerVersion[mostRecent], v.Name)
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
