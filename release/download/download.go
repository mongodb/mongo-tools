package download

import (
	"log"
	"regexp"
	"sort"
	"strconv"
)

// JSONFeed represents the structure of the JSON
// document consumed by the MongoDB downloads center.
// An abbreviated version of the document might look like:
//
//	{
//	  "versions": [
//	    {
//	      "version": "4.3.2",
//	      "downloads": [
//	        {
//	          "name": "amazon",
//	          "arch": "x86_64",
//	          "archive": {
//	            "url": "fastdl.mongodb.org/tools/db/...tgz",
//	            "md5": "4ec7...",
//	            "sha1": "3269...",
//	            "sha256": "0b679..."
//	          },
//	          "package": {
//	            "url": "fastdl.mongodb.org/tools/db/...rpm",
//	            "md5": "5b35...",
//	            "sha1": "b07c...",
//	            "sha256": "f6e7..."
//	          }
//	        },
//	        ...
//	      ]
//	    }
//	  ]
//	}
type JSONFeed struct {
	Versions []*ToolsVersion `json:"versions"`
}

type ToolsVersion struct {
	Version   string           `json:"version"`
	Downloads []*ToolsDownload `json:"downloads"`
}

type ToolsDownload struct {
	Name    string        `json:"name"`
	Arch    string        `json:"arch"`
	Archive ToolsArchive  `json:"archive"`
	Package *ToolsPackage `json:"package,omitempty"`
}

type ToolsArchive ToolsArtifact
type ToolsPackage ToolsArtifact

type ToolsArtifact struct {
	URL    string `json:"url"`
	Md5    string `json:"md5"`
	Sha1   string `json:"sha1"`
	Sha256 string `json:"sha256"`
}

func (f *JSONFeed) findOrCreateVersion(version string) *ToolsVersion {
	for _, v := range f.Versions {
		if v.Version == version {
			return v
		}
	}

	v := &ToolsVersion{Version: version}
	f.Versions = append(f.Versions, v)
	return v
}

// FindOrCreateDownload will find the ToolsDownload in f that matches the version, platform, and arch.
// If the ToolsDownload  does not exist, it will be added to f. If the parent ToolsVersion doesn't exist
// either, that will also be created.
func (f *JSONFeed) FindOrCreateDownload(version, platform, arch string) *ToolsDownload {
	v := f.findOrCreateVersion(version)

	for _, dl := range v.Downloads {
		if dl.Name == platform && dl.Arch == arch {
			return dl
		}
	}

	dl := &ToolsDownload{
		Name: platform,
		Arch: arch,
	}
	v.Downloads = append(v.Downloads, dl)
	return dl
}

// Sort will sort f.Versions by semver version. Version preference for the suffix of pre-release
// versions are alphabetical. Within each version, Downloads are sorted by OS name and architecture.
func (f *JSONFeed) Sort() {
	for _, v := range f.Versions {
		sort.Slice(v.Downloads, func(i, j int) bool {
			if v.Downloads[i].Name == v.Downloads[j].Name {
				return v.Downloads[i].Arch < v.Downloads[j].Arch
			}
			return v.Downloads[i].Name < v.Downloads[j].Name
		})
	}
	sort.Slice(f.Versions, func(i, j int) bool {
		return compareVersions(f.Versions[i].Version, f.Versions[j].Version)
	})
}

// compareVersions compares two semver version strings. version suffixes are compared alphabetiaclly.
func compareVersions(v1, v2 string) bool {
	versionParts := regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)-?(.*)$`)
	v1Parts := versionParts.FindStringSubmatch(v1)[1:]
	v2Parts := versionParts.FindStringSubmatch(v2)[1:]

	for i := range v1Parts {
		if v1Parts[i] != v2Parts[i] {
			if i == 3 {
				if v1Parts[i] == "" {
					return false
				}
				if v2Parts[i] == "" {
					return true
				}
				return v1Parts[i] < v2Parts[i]
			}

			v1Part, err := strconv.Atoi(v1Parts[i])
			if err != nil {
				log.Fatal(err)
			}

			v2Part, err := strconv.Atoi(v2Parts[i])
			if err != nil {
				log.Fatal(err)
			}

			return v1Part < v2Part
		}
	}

	// They're the same so just return true
	return true
}
