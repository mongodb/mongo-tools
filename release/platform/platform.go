package platform

import (
	"fmt"
	"os"
)

// Platform represents a platform (a combination of OS, distro,
// version, and architecture) on which we may build/test the tools.
// There should be at least one evergreen buildvariant per platform,
// and there may be multiple.
type Platform struct {
	Name string
	Arch string
}

const evgPlatformVar = "EVG_PLATFORM"

// Get returns the Platform for this host, based on the value of
// EVG_PLATFORM. It returns an error if EVG_PLATFORM is unset or set
// to an unknown value.
func Get() (Platform, error) {
	evgPlatform := os.Getenv(evgPlatformVar)
	if evgPlatform == "" {
		return Platform{}, fmt.Errorf("%s not set", evgPlatformVar)
	}

	pf, ok := platforms[evgPlatform]
	if !ok {
		return Platform{}, fmt.Errorf("unknown evg platform id %q", evgPlatform)
	}
	return pf, nil
}

// IsWindows returns true if the current host is a Windows host.
func IsWindows() (bool, error) {
	p, err := Get()
	if err != nil {
		return false, err
	}

	switch p.Name {
	case "win32":
		return true, nil
	default:
		return false, nil
	}
}

// IsLinux returns true if the current host is a Linux host.
func IsLinux() (bool, error) {
	p, err := Get()
	if err != nil {
		return false, err
	}

	switch p.Name {
	case "win32", "macos":
		return false, nil
	default:
		return true, nil
	}
}

func IsDeb(platformName string) bool {
	_, ok := debPlatformNames[platformName]
	return ok
}

func IsRPM(platformName string) bool {
	_, ok := rpmPlatformNames[platformName]
	return ok
}

func DebianArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	case "ppc64le":
		return "ppc64el"
	// other archs are the same name on Debian.
	default:
		return arch
	}
}

var platforms = map[string]Platform{
	"amazon1": {
		Name: "amazon1",
		Arch: "x86_64",
	},
	"amazon2": {
		Name: "amazon2",
		Arch: "x86_64",
	},
	"debian81": {
		Name: "debian8",
		Arch: "x86_64",
	},
	"debian92": {
		Name: "debian9",
		Arch: "x86_64",
	},
	"macos1014": {
		Name: "macos",
		Arch: "x86_64",
	},
	"rhel62": {
		Name: "rhel62",
		Arch: "x86_64",
	},
	"rhel70": {
		Name: "rhel70",
		Arch: "x86_64",
	},
	"suse12": {
		Name: "suse12",
		Arch: "x86_64",
	},
	"ubuntu1404": {
		Name: "ubuntu1404",
		Arch: "x86_64",
	},
	"ubuntu1604": {
		Name: "ubuntu1604",
		Arch: "x86_64",
	},
	"ubuntu1804": {
		Name: "ubuntu1804",
		Arch: "x86_64",
	},
	"windowsVS2017": {
		Name: "win32",
		Arch: "x86_64",
	},
	"ubuntu1604-arm": {
		Name: "ubuntu1604",
		Arch: "arm64",
	},
	"ubuntu1804-arm": {
		Name: "ubuntu1804",
		Arch: "arm64",
	},
	"rhel71-ppc": {
		Name: "rhel71",
		Arch: "ppc64le",
	},
	"ubuntu1604-ppc": {
		Name: "ubuntu1604",
		Arch: "ppc64le",
	},
	"ubuntu1804-ppc": {
		Name: "ubuntu1804",
		Arch: "ppc64le",
	},
	"rhel67-zseries": {
		Name: "rhel67",
		Arch: "s390x",
	},
	"rhel72-zseries": {
		Name: "rhel72",
		Arch: "s390x",
	},
	"ubuntu1604-zseries": {
		Name: "ubuntu1604",
		Arch: "s390x",
	},
	"ubuntu1804-zseries": {
		Name: "ubuntu1804",
		Arch: "s390x",
	},
}

var debPlatformNames = map[string]struct{}{
	"debian8":    {},
	"debian9":    {},
	"ubuntu1404": {},
	"ubuntu1604": {},
	"ubuntu1804": {},
}

var rpmPlatformNames = map[string]struct{}{
	"amazon1": {},
	"amazon2": {},
	"rhel62":  {},
	"rhel67":  {},
	"rhel70":  {},
	"rhel71":  {},
	"rhel72":  {},
	"suse12":  {},
}
