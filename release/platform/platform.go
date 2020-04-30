package platform

import (
	"fmt"

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
)

// Platform represents a platform (a combination of OS, distro,
// version, and architecture) on which we may build/test the tools.
// There should be at least one evergreen buildvariant per platform,
// and there may be multiple.
type Platform struct {
	Name  string
	Arch  string
	OS    string
	Pkg   string
	Repos []string
}

func (p Platform) Variant() string {
	if p.Arch == "x86_64" {
		return p.Name
	}
	return fmt.Sprintf("%s-%s", p.Name, p.Arch)
}

const evgVariantVar = "EVG_VARIANT"

// Get returns the Platform for this host, based on the value of
// EVG_VARIANT. It returns an error if EVG_VARIANT is unset or set
// to an unknown value.
func GetFromEnv() (Platform, error) {
	variant, err := env.EvgVariant()
	if err != nil {
		return Platform{}, err
	}

	pf, ok := GetByVariant(variant)
	if !ok {
		return Platform{}, fmt.Errorf("unknown evg variant %q", variant)
	}
	return pf, nil
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
	case "x86_64":
		return "amd64"
	case "ppc64le":
		return "ppc64el"
	// other archs are the same name on Debian.
	default:
		return p.Arch
	}
}

func (p Platform) ArtifactExtensions() []string {
	switch p.OS {
	case OSLinux:
		return []string{"tgz", "tgz.sig", p.Pkg}
	case OSMac:
		return []string{"tgz", "tgz.sig"}
	case OSWindows:
		return []string{"zip", "zip.sig", "msi"}
	}
	panic("unreachable")
}

var platformsByVariant map[string]Platform
var platforms = []Platform{
	{
		Name:  "amazon",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "amazon2",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "debian81",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "debian92",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "debian10",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name: "macos",
		Arch: "x86_64",
		OS:   OSMac,
	},
	{
		Name:  "rhel62",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "rhel70",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "rhel80",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "suse12",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "suse15",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1404",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1604",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1804",
		Arch:  "x86_64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name: "windows",
		Arch: "x86_64",
		OS:   OSWindows,
	},
	{
		Name:  "ubuntu1604",
		Arch:  "arm64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1804",
		Arch:  "arm64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "rhel71",
		Arch:  "ppc64le",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoEnterprise},
	},
	{
		Name:  "ubuntu1604",
		Arch:  "ppc64le",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1804",
		Arch:  "ppc64le",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "rhel67",
		Arch:  "s390x",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "rhel72",
		Arch:  "s390x",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "suse12",
		Arch:  "s390x",
		OS:    OSLinux,
		Pkg:   PkgRPM,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1604",
		Arch:  "s390x",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
	{
		Name:  "ubuntu1804",
		Arch:  "s390x",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoOrg, RepoEnterprise},
	},
}
