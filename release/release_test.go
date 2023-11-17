package main

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRepoConfig(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	type Repo struct {
		Name     string   `yaml:"name"`
		Type     string   `yaml:"type"`
		Edition  string   `yaml:"edition"`
		Bucket   string   `yaml:"bucket"`
		Repos    []string `yaml:"repos"`
		CodeName string   `yaml:"code_name,omitempty"`
	}

	type RepoConfig struct {
		Repos []Repo `yaml:"repos"`
	}

	filePath := "../etc/repo-config.yml"

	// Read the YAML file
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		require.NoError(t, err, "Error reading YAML file")
	}

	var repoConfig RepoConfig

	// Unmarshal the YAML data into the Config struct
	err = yaml.Unmarshal(yamlFile, &repoConfig)
	if err != nil {
		require.NoError(t, err, "Error parsing YAML")
	}

	for _, repo := range repoConfig.Repos {
		if repo.CodeName != "" {
			// If this test fails, it's possible that an entry in repo-config.yml has wrong repos.
			for _, repoRepo := range repo.Repos {
				require.Contains(t, repoRepo, repo.CodeName, "Repo does not contain code name")
			}
		}
	}
}
