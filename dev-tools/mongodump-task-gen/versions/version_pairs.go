package versions

import (
	"cmp"
	"fmt"
	"slices"

	mapset "github.com/deckarep/golang-set/v3"
)

type VersionPair struct {
	SrcVersion, DstVersion ServerVersion
}

// SupportedVersionPairs returns a slice of all supported version pairs, sorted first by source
// version and then by destination version, in ascending version order.
func SupportedVersionPairs() []VersionPair {
	var allPairs []VersionPair

	for src, dsts := range SupportedVersionPairsBySourceVersion {
		for _, dst := range dsts.ToSlice() {
			allPairs = append(allPairs, VersionPair{src, dst})
		}
	}

	slices.SortFunc(
		allPairs,
		func(a, b VersionPair) int {
			return cmp.Or(
				a.SrcVersion.Compare(b.SrcVersion),
				a.DstVersion.Compare(b.DstVersion),
			)
		},
	)

	return allPairs
}

func SupportedVersionPairsSet() mapset.Set[VersionPair] {
	return mapset.NewSet(SupportedVersionPairs()...)
}

// LatestVersionPair returns a `VersionPair` containing the latest version for both the source and
// destination versions. Sometimes, these two versions may not be equal, as we may add support for a
// new version as a destination before adding it as a source.
func LatestVersionPair() VersionPair {
	return VersionPair{Latest(), Latest()}
}

// Equals returns true if the two `VersionPair` values are the same.
func (vc VersionPair) Equals(other VersionPair) bool {
	return vc.SrcVersion.Equals(other.SrcVersion) && vc.DstVersion.Equals(other.DstVersion)
}

// Compare returns an integer for use in sorting functions.
func (vc VersionPair) Compare(other VersionPair) int {
	return cmp.Or(
		vc.SrcVersion.Compare(other.SrcVersion),
		vc.DstVersion.Compare(other.DstVersion),
	)
}

// For example: "v50-to-v50-amazon2-arm64".
func (vc VersionPair) Amazon2ARM64String() string {
	return fmt.Sprintf("%s-to-%s-amazon2-arm64", vc.SrcVersion.VString(), vc.DstVersion.VString())
}

// For example: v50-to-v50-amazon2-arm64 -> "v5.0 to v5.0 Amazon 2 ARM64".
func (vc VersionPair) Amazon2ARM64DisplayName() string {
	return fmt.Sprintf("v%s to v%s Amazon 2 ARM64", vc.SrcVersion.String(), vc.DstVersion.String())
}

func (vc VersionPair) UnderscoreString() string {
	return fmt.Sprintf("%s_to_%s", vc.SrcVersion.VString(), vc.DstVersion.VString())
}
