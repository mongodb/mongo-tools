package download

// JSONFeed represents the structure of the JSON
// document consumed by the MongoDB downloads center.
// An abbreviated version of the document might look like:
// {
//   "versions": [
//     {
//       "version": "4.3.2",
//       "downloads": [
//         {
//           "name": "amazon",
//           "arch": "x86_64",
//           "archive": {
//             "url": "fastdl.mongodb.org/tools/db/...tgz",
//             "md5": "4ec7...",
//             "sha1": "3269...",
//             "sha256": "0b679..."
//           },
//           "package": {
//             "url": "fastdl.mongodb.org/tools/db/...rpm",
//             "md5": "5b35...",
//             "sha1": "b07c...",
//             "sha256": "f6e7..."
//           }
//         },
//         ...
//       ]
//     }
//   ]
// }
type JSONFeed struct {
	Versions []ToolsVersion `json:"versions"`
}

type ToolsVersion struct {
	Version   string          `json:"version"`
	Downloads []ToolsDownload `json:"downloads"`
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
