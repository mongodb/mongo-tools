package download

import (
	"fmt"
	"strings"

	"github.com/mongodb/mongo-tools/release/platform"
	"github.com/mongodb/mongo-tools/release/version"
)

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
		if strings.Contains(v.Version, "-rc") {
			continue
		}

		feedVersion, err := version.Parse(v.Version)
		if err != nil {
			return "", "", "", fmt.Errorf("Unable to parse feed version: %v", err)
		}
		fmt.Printf("feedVersion: %+v\n", feedVersion)

		if serverVersion == "latest" ||
			(feedVersion.Major == sv.Major && feedVersion.Minor == sv.Minor) {
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
