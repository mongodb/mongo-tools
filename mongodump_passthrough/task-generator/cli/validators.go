package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/urfave/cli/v2"
)

func validateMongoVersionFlags(_ *cli.Context, requestedVersions []string) error {
	seen := mapset.NewSet[string]()
	bad := make([]string, 0, len(requestedVersions))

	for _, version := range requestedVersions {
		_, err := versions.ParseSupportedServerVersion(version)
		if err != nil {
			bad = append(bad, version)
		}

		if seen.Contains(version) {
			return fmt.Errorf(`bad --version: "%s" provided more than once`, version)
		}

		seen.Add(version)
	}

	if seen.Contains("all") {
		if len(requestedVersions) > 1 {
			return fmt.Errorf(`bad --version: you cannot use "all" with other versions`)
		}

		useAllMongoVersions = true
		return nil
	}

	if len(bad) > 0 {
		return fmt.Errorf("unknown --version: %s\nvalid versions: %s",
			strings.Join(bad, ", "),
			versions.SupportedVersionsString(),
		)
	}

	return nil
}

func validateTaskFlags(_ *cli.Context, tasks []string) error {
	for _, re := range tasks {
		_, err := regexp.Compile(re)
		if err != nil {
			return fmt.Errorf(`bad --task: could not compile "%s": %w`, re, err)
		}
	}

	return nil
}
