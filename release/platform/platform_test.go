package platform

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformsMatchCI(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	assert := assert.New(t)
	require := require.New(t)

	config, err := evergreen.Load()
	require.NoError(err)

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

func TestPlatformsHaveNoDuplicates(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	for _, p1 := range platforms {
		var dupes int
		for _, p2 := range platforms {
			if reflect.DeepEqual(p1, p2) {
				dupes++
			}
		}
		assert.Equal(t, 1, dupes, "platform %v only occurs once in platforms list", p1)
	}
}

func TestPlatformCorrectArch(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	for _, p := range platforms {
		name := p.Name
		if p.Arch == ArchArm64 || p.Arch == ArchAarch64 {
			if strings.Contains(name, "rhel") || strings.Contains(name, "amazon") || strings.Contains(name, "suse") {
				assert.Equal(t, ArchAarch64, p.Arch, "platform %v need arch %s", p, ArchAarch64)
			} else if strings.Contains(name, "debian") || strings.Contains(name, "ubuntu") {
				assert.Equal(t, ArchArm64, p.Arch, "platform %v need arch %s", p, ArchArm64)
			}
		}
	}
}
