package versions

import (
	"fmt"
	"regexp"
)

type pinnedVersions struct {
	Server  string
	Resmoke string
}

const (
	// Resmoke is downloaded with the commit determined by resmokePinV6 for server versions <=6.0
	// and the commit determined by resmokePinV7 for server version 7.0. The compiled server
	// binary commits are downloaded separately from the above, and are determined by the pin-generator.
	// To update this value, replace the resmoke pins below with a commit version from evergreen.
	// The latest commits can be found with the following links:
	// - V6: https://evergreen.mongodb.com/rest/v2/projects/mongodb-mongo-v6.0/versions
	// - V7: https://evergreen.mongodb.com/rest/v2/projects/mongodb-mongo-v7.0/versions
	// - V8: https://evergreen.mongodb.com/rest/v2/projects/mongodb-mongo-v8.0/versions
	//
	// The mongosync toolkit’s [Evergreen URL Getter](https://github.com/10gen/mongosync-toolkit/tree/main/evergreen-url-getter) is an easy way to send such API queries.
	//
	// The corresponding evergreen versions need to have successfully pushed binaries on
	// the platforms we're running tests on. Use the `version_id` field from the API output
	// as version pins. If the latest version doesn't work, it might be that its evergreen push
	// task hasn't finished. You could try selecting older ones from the links above.
	resmokePinV6 = "mongodb_mongo_v6.0_fc88ca137231d7457aed6265d4f32a361ae71716"
	resmokePinV7 = "mongodb_mongo_v7.0_ef5466e591fd7b01ac0edb16b1403c28daa04004"
	// V8 is not pinning a specific commit hash because we couldn't find one that is easily
	// downloadable with db-contrib-tools. We should consider either backport the s3 bucket
	// download methods here or unpin V6 and V7 as well in the future.
	resmokePinV8 = "8.0"

	// JstestfuzzPin is a commit arbitrarily chosen from the 10gen/jstestfuzz repo.
	// This pin should be updated regularly.
	JstestfuzzPin = "efb054c0a79b60c71fb32849fb3be7eaf1c7eb0a"

	// DriversEvergreenToolsPin is a commit arbitrarily chosen from the mongodb-labs/drivers-evergreen-tools repo.
	DriversEvergreenToolsPin = "e5b7e18b4773799bbe543d8bbb76f2276eea170e"
)

// GetServerPin returns the pinned server version that we use for testing.
func (v ServerVersion) GetServerPin() string {
	return v.getPins().Server
}

var versionRE = regexp.MustCompile(`^[1-9]+\.[0-9]+$`)

// This returns true if the pinned version is a release version as opposed to a specific git commit.
func (v ServerVersion) PinIsReleaseVersion() bool {
	return versionRE.MatchString(v.getPins().Server)
}

// GetServerPin returns the pinned resmoke version that we use for testing.
func (v ServerVersion) GetResmokePin() string {
	return v.getPins().Resmoke
}

func (v ServerVersion) getPins() pinnedVersions {
	pin, ok := serverVersionToPinnedVersions[v]
	if !ok {
		panic(fmt.Sprintf("No pin exists for server version %s!", v))
	}

	return pin
}
