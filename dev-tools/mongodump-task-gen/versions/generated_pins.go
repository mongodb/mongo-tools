// This file is generated via "go run cmd/pin-generator/main.go" and should
// not be edited manually except to set versions to a release version. See below for details.

package versions

// In general, we prefer to use the latest released Server version. To do that, the Server version
// should be something like "7.0" (with no patch number). This will use the latest released
// version. When a version is pinned to the release version, running the pin-generator tool will not
// update that pin.
//
// However, sometimes we need to pin a commit that's _newer_ than the most recent release. In those
// cases, the pin will be updated whenever the pin-generator tool is run. To unpin from the latest
// release version, you can manually change the Server version in the table below to an empty
// string.
var serverVersionToPinnedVersions = map[ServerVersion]pinnedVersions{
	V44: {Server: "4.4", Resmoke: resmokePinV6},
	V50: {
		Server:  "mongodb_mongo_v5.0_ba92303e18e7ed4701572aa15acd161c97796f2f",
		Resmoke: resmokePinV6,
	},
	V60: {Server: "6.0", Resmoke: resmokePinV6},
	V70: {Server: "7.0", Resmoke: resmokePinV7},
	V80: {Server: "8.0", Resmoke: resmokePinV8},
	V83: {Server: "8.3", Resmoke: resmokePinV8},
}
