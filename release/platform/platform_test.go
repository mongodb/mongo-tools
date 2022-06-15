package platform

import (
	"io/ioutil"
	"path/filepath"
	"runtime"
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
				name += "-" + p.Arch
			}
		}
		if p.UploadToS3Only {
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
