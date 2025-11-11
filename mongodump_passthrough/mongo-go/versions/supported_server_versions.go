package versions

import (
	"cmp"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
)

var (
	// V42 is here for historical purposes.
	// We no longer support server 4.2.
	V42 = ServerVersion{4, 2}

	V44 = ServerVersion{4, 4}
	V50 = ServerVersion{5, 0}
	V60 = ServerVersion{6, 0}
	V70 = ServerVersion{7, 0}
	V80 = ServerVersion{8, 0}

	SupportedVersionPairsBySourceVersion = map[ServerVersion]mapset.Set[ServerVersion]{
		V44: mapset.NewSet(V60),
		V50: mapset.NewSet(V60, V70),
		V60: mapset.NewSet(V60, V70, V80),
		V70: mapset.NewSet(V70, V80),
		V80: mapset.NewSet(V80),
	}
)

// Latest returns the latest-supported server version (source or destination).
func Latest() ServerVersion {
	serverVersions := SupportedVersions()
	slices.Reverse(serverVersions)

	return serverVersions[0]
}

// SupportedVersions returns a slice of all supported
// server versions (as either source or destination),
// sorted in ascending version order.
func SupportedVersions() []ServerVersion {
	allVersions := mapset.NewSet[ServerVersion]()

	for src, dsts := range SupportedVersionPairsBySourceVersion {
		allVersions.Add(src)
		allVersions.Append(dsts.ToSlice()...)
	}

	sorted := allVersions.ToSlice()
	slices.SortFunc(
		sorted,
		func(a, b ServerVersion) int {
			v := cmp.Compare(a.major, b.major)

			if v == 0 {
				v = cmp.Compare(a.minor, b.minor)
			}

			return v
		},
	)

	return sorted
}

// SupportedVersionsString returns a space-separated string of the
// SupportedVersions, suitable for use in an error message.
func SupportedVersionsString() string {
	sv := SupportedVersions()
	strs := make([]string, len(sv))
	for i, v := range sv {
		strs[i] = v.String()
	}

	return strings.Join(strs, " ")
}
