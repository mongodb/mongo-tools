//go:build unit

package main

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/stretchr/testify/require"
)

func TestCheckDownloadResponse(t *testing.T) {
	newResponse := func(status int, body string) *http.Response {
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}

	t.Run("a successful response is not an error", func(t *testing.T) {
		require.NoError(t, checkDownloadResponse(newResponse(http.StatusOK, "")))
	})

	t.Run("an S3 credential error explains that the credentials are at fault", func(t *testing.T) {
		err := checkDownloadResponse(
			newResponse(http.StatusForbidden, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>InvalidAccessKeyId</Code><Message>The AWS Access Key Id you provided does not exist in our records.</Message><RequestId>YPJR6F724JWN7314</RequestId></Error>`),
		)

		require.ErrorContains(t, err, "InvalidAccessKeyId", "reports the S3 error code")
		require.ErrorContains(
			t,
			err,
			"The AWS Access Key Id you provided does not exist in our records.",
			"reports the S3 error message",
		)
		require.ErrorContains(t, err, "YPJR6F724JWN7314", "reports the S3 request ID")
		require.ErrorContains(
			t,
			err,
			"problem with the credentials rather than with this build",
			"says that the credentials are at fault",
		)
	})

	t.Run("a non-credential S3 error does not blame the credentials", func(t *testing.T) {
		err := checkDownloadResponse(
			newResponse(http.StatusNotFound, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`),
		)

		require.ErrorContains(t, err, "NoSuchKey", "reports the S3 error code")
		require.NotContains(
			t,
			err.Error(),
			"problem with the credentials",
			"does not say that the credentials are at fault",
		)
	})

	t.Run("a response that is not S3 XML reports the status and body", func(t *testing.T) {
		err := checkDownloadResponse(newResponse(http.StatusBadGateway, "upstream is down"))

		require.ErrorContains(t, err, "Bad Gateway", "reports the HTTP status")
		require.ErrorContains(t, err, "upstream is down", "reports the response body")
	})
}

func TestRepoConfig(t *testing.T) {
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

func TestRepoConfigEntries(t *testing.T) {
	type Repo struct {
		Name    string `yaml:"name"`
		Edition string `yaml:"edition"`
	}

	type RepoConfig struct {
		Repos []Repo `yaml:"repos"`
	}

	yamlFile, err := os.ReadFile("../etc/repo-config.yml")
	require.NoError(t, err, "reading repo-config.yml")

	var repoConfig RepoConfig
	err = yaml.Unmarshal(yamlFile, &repoConfig)
	require.NoError(t, err, "parsing repo-config.yml")

	// The repo-config.yml entries are keyed by platform name and are consumed by curator via
	// `repo submit --distro <platform name>`, so a missing entry breaks the push task for that
	// platform.
	configEntries := map[string]map[string]bool{}
	for _, repo := range repoConfig.Repos {
		if configEntries[repo.Name] == nil {
			configEntries[repo.Name] = map[string]bool{}
		}
		configEntries[repo.Name][repo.Edition] = true
	}

	platformEditions := map[string]map[string]bool{}
	for _, p := range platform.Platforms() {
		// Platforms with SkipForJSONFeed set never run the push task, so they have no
		// repo-config.yml entries.
		if p.SkipForJSONFeed || len(p.Repos) == 0 {
			continue
		}
		if platformEditions[p.Name] == nil {
			platformEditions[p.Name] = map[string]bool{}
		}
		for _, repo := range p.Repos {
			platformEditions[p.Name][string(repo)] = true
		}
	}

	for name, editions := range platformEditions {
		for edition := range editions {
			require.Containsf(
				t,
				configEntries[name],
				edition,
				"the %s platform needs a repo-config.yml entry with edition %s",
				name,
				edition,
			)
		}
	}

	for name := range configEntries {
		_, exists := platformEditions[name]
		require.Truef(
			t,
			exists,
			"repo-config.yml has an entry for %s but there is no platform with that name",
			name,
		)
	}
}
