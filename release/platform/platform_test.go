package platform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type EvergreenConfig struct {
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

func TestPlatformsMatchCI(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	assert := assert.New(t)
	require := require.New(t)

	_, testPath, _, _ := runtime.Caller(0)
	common, err := ioutil.ReadFile(filepath.Join(filepath.Dir(testPath), "..", "..", "common.yml"))
	require.NoError(err)

	var config EvergreenConfig
	yaml.Unmarshal(common, &config)

	releasePlatforms := make(map[string]bool)
	s3OnlyPlatforms := make(map[string]bool)
	for _, p := range platforms {
		name := p.VariantName
		if name == "" {
			name = p.Name
			if p.Arch != ArchX86_64 {
				name += "-" + string(p.Arch)
			}
		}
		if p.SkipForJSONFeed {
			s3OnlyPlatforms[name] = false
		} else {
			releasePlatforms[name] = false
		}
	}

	for _, v := range config.Variants {
		if v.Name == "release" || v.Name == "static" || v.Name == "ubuntu-race" {
			continue
		}

		if _, ok := releasePlatforms[v.Name]; ok {
			releasePlatforms[v.Name] = true
		} else if _, ok := s3OnlyPlatforms[v.Name]; ok {
			for _, t := range v.Tasks {
				assert.NotEqual(t.Name, "push", "s3-only buildvariants should not include the push task")
			}
		} else {
			assert.Fail(
				"missing platform",
				"%s (%s) is in the evergreen config but is not in the list of release platforms",
				v.Name,
				v.DisplayName,
			)
		}
	}

	for name, seen := range releasePlatforms {
		assert.True(seen, "%s from the list of known platforms is in the evergreen config", name)
	}
}

func TestPlatformsAreSorted(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	current := Platforms()
	var sorted []Platform
	for _, p := range current {
		sorted = append(sorted, p)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		if sorted[i].OS != sorted[j].OS {
			return sorted[i].OS < sorted[j].OS
		}
		if sorted[i].Arch != sorted[j].Arch {
			return sorted[i].Arch < sorted[j].Arch
		}

		tagsI := sorted[i].BuildTags
		tagsJ := sorted[j].BuildTags
		sort.Strings(tagsI)
		sort.Strings(tagsJ)

		// We need to join on a string that will never appear in the tags. An
		// emoji seems like a safe bet.
		return strings.Join(tagsI, "❤") < strings.Join(tagsJ, "❤")
	})
	if !assert.Equal(t, sorted, current) {
		var golang []string
		for _, p := range sorted {
			golang = append(golang, p.asGolangString())
		}
		fmt.Println("Sorted platforms:")
		fmt.Printf("%s,\n", strings.Join(golang, ","))
	}
}
