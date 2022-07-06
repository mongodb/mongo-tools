package platform

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mongodb/mongo-tools/release/env"
)

const (
	OSWindows = "windows"
	OSLinux   = "linux"
	OSMac     = "mac"

	PkgDeb = "deb"
	PkgRPM = "rpm"

	RepoOrg        = "org"
	RepoEnterprise = "enterprise"

	ArchArm64   = "arm64"
	ArchS390x   = "s390x"
	ArchPpc64le = "ppc64le"
	ArchX86_64  = "x86_64"
)

// Platform represents a platform (a combination of OS, distro,
// version, and architecture) on which we may build/test the tools.
// There should be at least one evergreen buildvariant per platform,
// and there may be multiple.
type Platform struct {
	Name      string
	Arch      string
	OS        string
	Pkg       string
	Repos     []string
	BuildTags []string
	BinaryExt string
}

func (p Platform) Variant() string {
	if p.Arch == ArchX86_64 {
		return p.Name
	}
	return fmt.Sprintf("%s-%s", p.Name, p.Arch)
}

const evgVariantVar = "EVG_VARIANT"

// GetFromEnv returns the Platform for this host, based on the value
// of EVG_VARIANT. It returns an error if EVG_VARIANT is unset or set
// to an unknown value.
func GetFromEnv() (Platform, error) {
	variant, err := env.EvgVariant()
	if err != nil {
		return Platform{}, err
	}

	if variant == "ubuntu-race" {
		variant = "ubuntu1804"
	}

	pf, ok := GetByVariant(variant)
	if !ok {
		return Platform{}, fmt.Errorf("unknown evg variant %q", variant)
	}
	return pf, nil
}

// DetectLocal detects the platform for non-evergreen use cases.
func DetectLocal() (Platform, error) {
	cmd := exec.Command("uname", "-s")
	out, err := cmd.Output()
	if err != nil {
		return Platform{}, fmt.Errorf("failed to run uname: %w", err)
	}
	kernelName := strings.TrimSpace(string(out))

	if strings.HasPrefix(kernelName, "CYGWIN") {
		pf, ok := GetByVariant("windows")
		if !ok {
			panic("windows platform name changed")
		}
		return pf, nil
	}

	switch kernelName {
	case "Linux":
		pf, ok := GetByVariant("ubuntu1804")
		if !ok {
			panic("ubuntu1804 platform name changed")
		}
		return pf, nil
	case "Darwin":
		pf, ok := GetByVariant("macos")
		if !ok {
			panic("macos platform name changed")
		}
		return pf, nil
	}

	return Platform{}, fmt.Errorf("failed to detect local platform from kernel name %q", kernelName)
}

func GetByVariant(variant string) (Platform, bool) {
	if platformsByVariant == nil {
		platformsByVariant = make(map[string]Platform)
		for _, p := range platforms {
			platformsByVariant[p.Variant()] = p
		}
	}
	p, ok := platformsByVariant[variant]
	return p, ok
}

func Count() int {
	return len(platforms)
}

func (p Platform) DebianArch() string {
	if p.Pkg != PkgDeb {
		panic("called DebianArch on non-debian platform")
	}
	switch p.Arch {
	case ArchX86_64:
		return "amd64"
	case ArchPpc64le:
		return "ppc64el"
	// other archs are the same name on Debian.
	default:
		return p.Arch
	}
}

func (p Platform) RPMArch() string {
	if p.Pkg != PkgRPM {
		panic("called RPMArch on non-rpm platform")
	}
	switch p.Arch {
	case ArchArm64:
		return "aarch64"
	// other archs are the same name on RPM.
	default:
		return p.Arch
	}
}

func (p Platform) ArtifactExtensions() []string {
	switch p.OS {
	case OSLinux:
		return []string{"tgz", "tgz.sig", p.Pkg}
	case OSMac:
		return []string{"zip"}
	case OSWindows:
		return []string{"zip", "zip.sig", "msi"}
	}
	panic("unreachable")
}

var platformsByVariant map[string]Platform
var defaultBuildTags = []string{"ssl", "sasl", "gssapi", "failpoints"}
var platforms = []Platform{
	{
		Name:      "amazon",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "amazon2",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "amazon2",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian81",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian92",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian10",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian11",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "macos",
		Arch:      ArchX86_64,
		OS:        OSMac,
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel62",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel70",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel71",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel72",
		Arch:      ArchS390x,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel80",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel81",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel82",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "suse12",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "suse15",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1604",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: []string{"ssl", "failpoints"},
	},
	{
		Name:      "ubuntu1604",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1604",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1804",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: []string{"ssl", "failpoints"},
	},
	{
		Name:      "ubuntu1804",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1804",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu2004",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: []string{"ssl", "failpoints"},
	},
	{
		Name:      "ubuntu2004",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu2204",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: []string{"ssl", "failpoints"},
	},
	{
		Name:      "ubuntu2204",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []string{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "windows",
		Arch:      ArchX86_64,
		OS:        OSWindows,
		BuildTags: defaultBuildTags,
		BinaryExt: ".exe",
	},
	{
		Name:      "rhel83",
		Arch:      ArchS390x,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []string{RepoEnterprise},
		BuildTags: []string{"ssl", "sasl", "gssapi", "failpoints"},
	},
}

func Platforms() []Platform {
	return platforms
}
