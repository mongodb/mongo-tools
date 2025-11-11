package generate

import (
	"fmt"
	"slices"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
)

type VersionCombination struct {
	srcVersion, dstVersion versions.ServerVersion
}

var SupportedVersionCombinations = []VersionCombination{}

func init() {
	for srcVersion, dstVersions := range versions.SupportedVersionPairsBySourceVersion {
		for _, dstVersion := range dstVersions.ToSlice() {
			bv := VersionCombination{srcVersion, dstVersion}
			SupportedVersionCombinations = append(SupportedVersionCombinations, bv)
		}
	}

	// Sorting these ensures that the code to skip some version combinations always skips the same
	// combinations when given the same seed.
	slices.SortFunc(
		SupportedVersionCombinations,
		func(a, b VersionCombination) int {
			if a.srcVersion != b.srcVersion {
				return a.srcVersion.Compare(b.srcVersion)
			}

			return a.dstVersion.Compare(b.dstVersion)
		},
	)
}

// Override the SupportedVersionCombinations for mongodump-task-gen.
func initVersionCombinationsForMongodump() {
	SupportedVersionCombinations = MongodumpVersionCombinations
}

// MongodumpVersionCombinations are test combinations for mongodump without oplog replay.
var MongodumpVersionCombinations = []VersionCombination{
	// TODO: support version combinations involving V42 (against V42 through to V80)

	//{versions.V44, versions.V44},
	//{versions.V44, versions.V50},
	//{versions.V44, versions.V60},
	//{versions.V44, versions.V70},
	//{versions.V44, versions.V80},
	//
	//{versions.V50, versions.V50},
	//{versions.V50, versions.V60},
	//{versions.V50, versions.V70},
	//{versions.V50, versions.V80},
	//
	//{versions.V60, versions.V60},
	//{versions.V60, versions.V70},
	//{versions.V60, versions.V80},
	//
	//{versions.V70, versions.V70},
	//{versions.V70, versions.V80},
	//
	{versions.V80, versions.V80},
}

// MongodumpWithOplogVersionCombinations are test combinations for mongodump with oplog replay.
var MongodumpWithOplogVersionCombinations = []VersionCombination{
	//{versions.V42, versions.V42},
	//{versions.V42, versions.V44},

	//{versions.V44, versions.V44},
	//{versions.V44, versions.V50},
	//
	//{versions.V50, versions.V50},
	//{versions.V50, versions.V60},
	//
	//{versions.V60, versions.V60},
	//{versions.V60, versions.V70},
	//
	//{versions.V70, versions.V70},
	//{versions.V70, versions.V80},

	{versions.V80, versions.V80},
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
