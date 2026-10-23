package generate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mongodb/mongo-tools/dev-tools/mongodump-task-gen/versions"
)

type VersionCombination struct {
	srcVersion, dstVersion versions.ServerVersion
}

var SupportedVersionCombinations = MongodumpVersionCombinations

// MongodumpVersionCombinations are test combinations for mongodump without oplog replay.
var MongodumpVersionCombinations = []VersionCombination{
	// TODO: support version combinations involving V42 (against V42 through to V80)

	{versions.V44, versions.V44},
	{versions.V44, versions.V50},
	{versions.V44, versions.V60},
	{versions.V44, versions.V70},
	{versions.V44, versions.V80},

	{versions.V50, versions.V50},
	{versions.V50, versions.V60},
	{versions.V50, versions.V70},
	{versions.V50, versions.V80},

	{versions.V60, versions.V60},
	{versions.V60, versions.V70},
	{versions.V60, versions.V80},

	{versions.V70, versions.V70},
	{versions.V70, versions.V80},

	{versions.V80, versions.V80},
	{versions.V83, versions.V83},
}

// MongodumpWithOplogVersionCombinations are test combinations for mongodump with oplog replay.
var MongodumpWithOplogVersionCombinations = []VersionCombination{
	//{versions.V42, versions.V42},
	//{versions.V42, versions.V44},

	{versions.V44, versions.V44},
	{versions.V44, versions.V50},

	{versions.V50, versions.V50},
	{versions.V50, versions.V60},

	{versions.V60, versions.V60},
	{versions.V60, versions.V70},

	{versions.V70, versions.V70},
	{versions.V70, versions.V80},

	{versions.V80, versions.V80},
	{versions.V83, versions.V83},
}

// Does mongodump without oplog support this VersionCombination ?
func (vc VersionCombination) IsMongodumpSupported() bool {
	return slices.Contains(MongodumpVersionCombinations, vc)
}

// Does mongodump with oplog support this VersionCombination ?
func (vc VersionCombination) IsMongodumpWithOplogSupported() bool {
	return slices.Contains(MongodumpWithOplogVersionCombinations, vc)
}

// For example: "v50-to-v50-amazon2-arm64".
func (vc VersionCombination) Amazon2ARM64String() string {
	return fmt.Sprintf("%s-to-%s-amazon2-arm64", vc.srcVersion.VString(), vc.dstVersion.VString())
}

// For example: v50-to-v50-amazon2-arm64 -> "v5.0 to v5.0 Amazon2 ARM 64".
func (vc VersionCombination) Amazon2ARM64DisplayName() string {
	return fmt.Sprintf("v%s to v%s Amazon2 ARM64", vc.srcVersion.String(), vc.dstVersion.String())
}

func IsSupportedVersion(v versions.ServerVersion) bool {
	for _, vc := range SupportedVersionCombinations {
		if v == vc.srcVersion || v == vc.dstVersion {
			return true
		}
	}

	return false
}

func SupportedVersionsString() string {
	var strs []string
	for _, vc := range SupportedVersionCombinations {
		s := vc.srcVersion.String()
		if !slices.Contains(strs, s) {
			strs = append(strs, s)
		}
	}

	return strings.Join(strs, " ")
}

func suiteNameWithVersionCombinationSuffix(str string, vc VersionCombination) string {
	return appendVersionCombination(str, vc)
}

func taskNameWithVersionCombination(
	str string,
	vc VersionCombination,
) string {
	return appendVersionCombination(str, vc)
}

func appendVersionCombination(str string, vc VersionCombination) string {
	return fmt.Sprintf("%s_%s_to_%s", str, vc.srcVersion.VString(), vc.dstVersion.VString())
}
