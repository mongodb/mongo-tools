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
//
// Note: Update 5.0 Server to mongodb_mongo_v5.0_a0cc84ed090350d63ca63501a474dbf1efd02d08 if you
// have issues with running 5.0 test clusters on macOS. This is because mongodb_mongo_v5.0_a75f506a2bf864d24577369d3c98d98eac14bfb3 doesn't have any macOS builds (removed in https://github.com/10gen/mongo/commit/03ee16bbb7afbced6578acf829fb15dc1584f431).
var serverVersionToPinnedVersions = map[ServerVersion]pinnedVersions{
	V44: {Server: "4.4", Resmoke: resmokePinV6},
	V50: {Server: "mongodb_mongo_v5.0_a75f506a2bf864d24577369d3c98d98eac14bfb3", Resmoke: resmokePinV6},
	V60: {Server: "6.0", Resmoke: resmokePinV6},
	V70: {Server: "7.0", Resmoke: resmokePinV7},
	V80: {Server: "8.0", Resmoke: resmokePinV8},
}
