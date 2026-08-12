package download

import (
	"fmt"
	"strings"

	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/mongodb/mongo-tools/release/version"
)

// Increase this as needed for new server versions.
var maxServerVersion = version.Version{9, 0, 999, ""}

// JSONFeed represents the structure of the JSON
// document consumed by the MongoDB downloads center.
type ServerJSONFeed struct {
	Versions []*ServerVersion `json:"versions"`
}

type ServerVersion struct {
	Version   string            `json:"version"`
	Downloads []*ServerDownload `json:"downloads"`
	GitHash   string            `json:"githash"`
}

type ServerDownload struct {
	Target  string        `json:"target"`
	Edition string        `json:"edition"`
	Arch    string        `json:"arch"`
	Archive ServerArchive `json:"archive"`
}

type ServerArchive struct {
	URL string `json:"url"`
}

var (
	ServerURLMissingError = fmt.Errorf(
		"Unable to find download URL for the server in the json feed",
	)
)

func (f *ServerJSONFeed) FindURLHashAndVersion(
	serverVersion string,
	platform platform.Platform,
	edition string,
) (string, string, string, error) {
	fmt.Printf("Finding %v, %v, %v\n", serverVersion, platform, edition)

	var sv version.Version
	var err error
	if serverVersion != "latest" {
		sv, err = version.Parse(serverVersion)
		if err != nil {
			return "", "", "", fmt.Errorf("Unable to parse server version: %v", err)
		}
		fmt.Printf("sv: %+v\n", sv)
	}

	// Return a version string that matches the specified major and minor number even if it cannot find an exact feed
	// satisfying all conditions.
	// This is useful to find a server release that is not in the feed.
	versionGuess := ""
	for _, v := range f.Versions {
		// Most of the time we want to skip release candidates in the feed, so that we test (for
		// example) 8.0.5 rather than 8.0.6-rc1. For adding new version support, though, we often want
		// to explicitly test against the latest rc, so we also check for that here.
		releaseCandidateOk := serverVersion == v.Version || serverVersion == "latest"
		if strings.Contains(v.Version, "-rc") && !releaseCandidateOk {
			fmt.Printf("Skipping release candidate: %v\n", v.Version)
			continue
		}

		feedVersion, err := version.Parse(v.Version)
		if err != nil {
			return "", "", "", fmt.Errorf("Unable to parse feed version: %v", err)
		}
		fmt.Printf("feedVersion: %+v (%s)\n", feedVersion, v.Version)

		if serverVersion == "latest" ||
			(feedVersion.Major == sv.Major && feedVersion.Minor == sv.Minor) {

			if feedVersion.GreaterThan(maxServerVersion) {
				if serverVersion == "latest" {
					fmt.Printf(
						"Skipping because it exceeds the max server version (%v)\n",
						maxServerVersion,
					)
					continue
				} else {
					return "", "", "", fmt.Errorf("cannot match a server version (%v) that exceeds the max server version (%v)", feedVersion, maxServerVersion)
				}
			}

			if versionGuess == "" {
				versionGuess = feedVersion.String()
			}
			for _, dl := range v.Downloads {
				if !platform.TargetMatches(dl.Target) {
					continue
				}

				if dl.Arch == platform.Arch.String() && dl.Edition == edition {
					return dl.Archive.URL, v.GitHash, v.Version, nil
				}
			}
		}
	}

	return "", "", versionGuess, ServerURLMissingError
}

// CandidateVersions returns every version in the feed matching the same criteria as
// FindURLHashAndVersion, in feed order (newest first), up to a maximum of limit versions.
//
// FindURLHashAndVersion only returns the first match, which is what we want for the server
// tarball. Some artifacts (notably the jstestshell) are published separately from the feed and may
// be missing for that particular patch release, so callers that can tolerate a version mismatch
// use this to fall back to an older patch release.
func (f *ServerJSONFeed) CandidateVersions(
	serverVersion string,
	platform platform.Platform,
	edition string,
	limit int,
) []string {
	var sv version.Version
	if serverVersion != "latest" {
		var err error
		sv, err = version.Parse(serverVersion)
		if err != nil {
			return nil
		}
	}

	var candidates []string
	for _, v := range f.Versions {
		if len(candidates) >= limit {
			break
		}

		releaseCandidateOk := serverVersion == v.Version || serverVersion == "latest"
		if strings.Contains(v.Version, "-rc") && !releaseCandidateOk {
			continue
		}

		feedVersion, err := version.Parse(v.Version)
		if err != nil {
			continue
		}

		if serverVersion != "latest" &&
			(feedVersion.Major != sv.Major || feedVersion.Minor != sv.Minor) {
			continue
		}

		if feedVersion.GreaterThan(maxServerVersion) {
			continue
		}

		for _, dl := range v.Downloads {
			if !platform.TargetMatches(dl.Target) {
				continue
			}

			if dl.Arch == platform.Arch.String() && dl.Edition == edition {
				candidates = append(candidates, v.Version)
				break
			}
		}
	}

	return candidates
}
