package db

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Version [3]int

func (v1 Version) Cmp(v2 Version) int {
	for i := range v1 {
		if v1[i] < v2[i] {
			return -1
		}
		if v1[i] > v2[i] {
			return 1
		}
	}
	return 0
}

// CmpMinor is like Cmp but only compares up to the minor version.
// It ignores the patch release version. For example, this function
// considers 4.4.0 and 4.4.1 to be “equivalent”, while 4.2.0 and 4.4.0
// differ.
func (v1 Version) CmpMinor(v2 Version) int {
	return cmp.Or(
		cmp.Compare(v1[0], v2[0]),
		cmp.Compare(v1[1], v2[1]),
	)
}

func (v1 Version) LT(v2 Version) bool {
	return v1.Cmp(v2) == -1
}

func (v1 Version) LTE(v2 Version) bool {
	return v1.Cmp(v2) != 1
}

func (v1 Version) GT(v2 Version) bool {
	return v1.Cmp(v2) == 1
}

func (v1 Version) GTE(v2 Version) bool {
	return v1.Cmp(v2) != -1
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

func StrToVersion(v string) (Version, error) {
	// get rid of build strings
	v = strings.SplitN(v, "-", 2)[0]
	v = strings.SplitN(v, "+", 2)[0]

	parts := strings.SplitN(v, ".", 3)

	if len(parts) != 3 {
		return Version{}, errors.New("invalid version string")
	}

	result := make([]int, 3)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, errors.New(
				"failed to parse version number part, invalid version strong",
			)
		}
		result[i] = n
	}
	return Version{result[0], result[1], result[2]}, nil
}
