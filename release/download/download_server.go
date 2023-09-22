package download

import (
	"fmt"
)

// JSONFeed represents the structure of the JSON
// document consumed by the MongoDB downloads center.
type ServerJSONFeed struct {
	Versions []*ServerVersion `json:"versions"`
}

type ServerVersion struct {
	Version   string            `json:"version"`
	Downloads []*ServerDownload `json:"downloads"`
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

func (f *ServerJSONFeed) FindURL(version string, target string, arch string, edition string) (string, error) {
	for _, v := range f.Versions {
		if v.Version == version {
			for _, dl := range v.Downloads {
				if dl.Target == target && dl.Arch == arch && dl.Edition == edition {
					return dl.Archive.URL, nil
				}
			}
		}
	}

	return "", fmt.Errorf("Unable to find download URL for the server in the json feed")
}
