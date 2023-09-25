package download

import (
	"fmt"

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

func (f *ServerJSONFeed) FindURLHashAndVersion(serverVersion string, target string, arch string, edition string) (string, string, string, error) {
	fmt.Printf("Finding %v, %v, %v, %v\n", serverVersion, target, arch, edition)

	var sv version.Version
	var err error
	if serverVersion != "latest" {
		sv, err = version.Parse(serverVersion)
		if err != nil {
			return "", "", "", fmt.Errorf("Unable to parse server version: %v", err)
		}
		fmt.Printf("sv: %+v\n", sv)
	}

	for _, v := range f.Versions {
		feedVersion, err := version.Parse(v.Version)
		if err != nil {
			return "", "", "", fmt.Errorf("Unable to parse feed version: %v", err)
		}
		fmt.Printf("feedVersion: %+v\n", feedVersion)

		if serverVersion == "latest" || (feedVersion.Major == sv.Major && feedVersion.Minor == sv.Minor) {
			for _, dl := range v.Downloads {
				if dl.Target == target && dl.Arch == arch && dl.Edition == edition {
					return dl.Archive.URL, v.GitHash, v.Version, nil
				}
			}
		}
	}

	return "", "", "", fmt.Errorf("Unable to find download URL for the server in the json feed")
}
