package main

import (
	"net/http"
	"net/http/httptest"
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

func TestTryDownloadBinaries(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// tryDownloadBinaries creates its temp dir under "bin".
	setup := func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.Mkdir("bin", 0700))
	}

	t.Run("non-200 response reports the status and body", func(t *testing.T) {
		setup(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<?xml version=\"1.0\"?><Error><Code>AccessDenied</Code></Error>"))
		}))
		defer srv.Close()

		err := tryDownloadBinaries(srv.URL + "/dist-test.tgz")
		require.Error(t, err)
		// The old code wrote the error document to the .tgz and only failed later with
		// "gzip: invalid header", which said nothing about the real problem.
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "AccessDenied")
		require.NotContains(t, err.Error(), "gzip")
	})

	t.Run("unexpected extension is rejected", func(t *testing.T) {
		setup(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not an archive"))
		}))
		defer srv.Close()

		err := tryDownloadBinaries(srv.URL + "/dist-test.txt")
		require.Error(t, err)
		require.Contains(t, err.Error(), "end in .zip or .tgz")
	})
}
