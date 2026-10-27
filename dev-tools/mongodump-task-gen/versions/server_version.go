package versions

import (
	"cmp"
	"encoding"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// ServerVersion represents a major and minor version combination for the server. This should
// be used as the canonical way to represent server versions.
//
// Only the `Unmarshal*` methods take a pointer, which they need to do since they populate the
// struct given as the receiver.
//
//nolint:recvcheck
type ServerVersion struct {
	major, minor int
}

var _ encoding.TextMarshaler = &ServerVersion{}
var _ encoding.TextUnmarshaler = &ServerVersion{}

// ParseSupportedServerVersion takes a string which contains a Server version like "6.0" or "8.0"
// and returns a `ServerVersion`. This method only works with supported Server versions. If you need
// to parse any `major.minor,*` version, use `ParseAnyVersion`.
func ParseSupportedServerVersion(version string) (ServerVersion, error) {
	if version == "latest" {
		return Latest(), nil
	}

	// This isn't really "parsing", but it's a simple way to match the given string against a known,
	// supported version.
	for _, v := range SupportedVersions() {
		if v.String() == version {
			return v, nil
		}
	}

	return ServerVersion{}, fmt.Errorf(
		"the provided version, %#q, is not a valid, supported Server version (supported versions: %s)",
		version,
		SupportedVersionsString(),
	)
}

const serverVersionParseErrorStr = `failed to parse %#q as ServerVersion:` +
	` version must have the format: major.minor(.patch)`

// ParseAnyServerVersion creates a Server version from a string of the format "major.minor.*". Don't
// use this if you need to parse a Server version that must be one of our supported versions! Use
// `ParseSupportedServerVersion` instead. Only use this when you need to handle any Server version,
// for example when parsing the FCV value.
func ParseAnyServerVersion(str string) (ServerVersion, error) {
	split := strings.Split(str, ".")
	if len(split) < 2 {
		return ServerVersion{}, errors.Errorf(serverVersionParseErrorStr, str)
	}
	major, err := strconv.Atoi(split[0])
	if err != nil {
		return ServerVersion{}, errors.Wrapf(err, serverVersionParseErrorStr, str)
	}
	minor, err := strconv.Atoi(split[1])
	if err != nil {
		return ServerVersion{}, errors.Wrapf(err, serverVersionParseErrorStr, str)
	}
	return ServerVersion{major, minor}, nil
}

// FromSlice creates a server version from an integer slice. Note: Don't use this in new code. This
// mostly exists because this package somewhat duplicates code in the `buildinfo` package.
func FromSlice(arr []int) ServerVersion {
	return ServerVersion{arr[0], arr[1]}
}

func (v ServerVersion) IsEmpty() bool {
	return v == ServerVersion{}
}

// AtLeast determines if the given server version is greater than or equal to the
// current server version.
func (v ServerVersion) AtLeast(other ServerVersion) bool {
	return ServerVersionAtLeast([]int{v.major, v.minor}, other)
}

func (v ServerVersion) LessThan(other ServerVersion) bool {
	//nolint:gocritic // this is the method that implements the lint suggestion
	return !v.AtLeast(other)
}

// Compare implements the standard `Compare` API for the `ServerVersion` type.
func (v ServerVersion) Compare(other ServerVersion) int {
	if v.major != other.major {
		return cmp.Compare(v.major, other.major)
	}
	return cmp.Compare(v.minor, other.minor)
}

// Equals returns true if the two versions represent the same Server version, ignoring their patch
// version.
func (v ServerVersion) Equals(other ServerVersion) bool {
	return v.Compare(other) == 0
}

// VString returns a v-prefixed string, e.g. "v50".
func (v ServerVersion) VString() string {
	return fmt.Sprintf("v%d%d", v.major, v.minor)
}

// String returns a decimal string, e.g. "5.0".
func (v ServerVersion) String() string {
	return fmt.Sprintf("%d.%d", v.major, v.minor)
}

// StringNoDot returns a string without dot, e.g. "50".
func (v ServerVersion) StringNoDot() string {
	return fmt.Sprintf("%d%d", v.major, v.minor)
}

// IsSupported returns whether this version is supported by mongosync.
func (v ServerVersion) IsSupported() bool {
	return slices.Contains(SupportedVersions(), v)
}

// MarshalText implements encoding.TextMarshaler, which means that we encode
// versions as strings for JSON.
func (v ServerVersion) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, which means that we
// decode versions as strings from JSON.
func (v *ServerVersion) UnmarshalText(text []byte) error {
	version, err := ParseSupportedServerVersion(string(text))
	if err != nil {
		return err
	}

	*v = version
	return err
}

// ServerVersionAtLeast determines if version1 is greater than or equal to the version
// represented by version0.
func ServerVersionAtLeast(version0 []int, version1 ServerVersion) bool {
	return checkVersionAtLeast(version0, []int{version1.major, version1.minor})
}

// VersionAtLeast determines if the version represented by version1 is greater than or
// equal to the version represented by version0.
func VersionAtLeast(version0, version1 []int) bool {
	return checkVersionAtLeast(version0, version1)
}

func checkVersionAtLeast(version0, version1 []int) bool {
	for i, version1Level := range version1 {
		if i == len(version0) {
			return false
		}
		if version0Level := version0[i]; version0Level != version1Level {
			return version0Level >= version1Level
		}
	}
	return true
}
