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
	resmokePinV6 = "mongodb_mongo_v6.0_919f50c37ef9f544a27e7c6e2d5e8e0093bc4902"
	resmokePinV7 = "mongodb_mongo_v7.0_34620e93b9a5d12d9f4848c0d26f101cfc74f1f7"
	resmokePinV8 = "mongodb_mongo_v8.0_d36db09e3b2fb813b1b51a677d67888e6f5d60ce"
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
