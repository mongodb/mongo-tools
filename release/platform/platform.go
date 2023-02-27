package platform

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/mongodb/mongo-tools/release/env"
)

type OS string

const (
	OSWindows OS = "windows"
	OSLinux      = "linux"
	OSMac        = "mac"
)

type Pkg string

const (
	PkgDeb Pkg = "deb"
	PkgRPM     = "rpm"
)

type Repo string

const (
	RepoOrg        Repo = "org"
	RepoEnterprise      = "enterprise"
)

type Arch string

const (
	ArchArm64 Arch = "arm64"
	// While arm64 and aarch64 are the same architecture, some Linux distros
	// use arm64 (Debian and RHEL) and others use aarch64 (Amazon 2).
	ArchAarch64 Arch = "aarch64"
	ArchS390x        = "s390x"
	ArchPpc64le      = "ppc64le"
	ArchX86_64       = "x86_64"
)

// Platform represents a platform (a combination of OS, distro,
// version, and architecture) on which we may build/test the tools.
// There should be at least one evergreen buildvariant per platform,
// and there may be multiple.
type Platform struct {
	Name string
	// This is used to override the variant name. It should only be used for
	// special builds. In general, we want to use the OS name + arch for the
	// variant name.
	VariantName     string
	Arch            Arch
	OS              OS
	Pkg             Pkg
	Repos           []Repo
	BuildTags       []string
	BinaryExt       string
	SkipForJSONFeed bool
}

func (p Platform) Variant() string {
	if p.VariantName != "" {
		return p.VariantName
	}
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

// CountForReleaseJSON returns the number of platforms that we expect to put into the release
// JSON. This is all platforms _except_ those where the `SkipForJSONFeed` field is true.
func CountForReleaseJSON() int {
	count := 0
	for _, p := range platforms {
		if p.SkipForJSONFeed {
			continue
		}
		count++
	}
	return count
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
		return p.Arch.String()
	}
}

func (p Platform) RPMArch() string {
	if p.Pkg != PkgRPM {
		panic("called RPMArch on non-rpm platform")
	}
	return p.Arch.String()
}

func (p Platform) ArtifactExtensions() []string {
	switch p.OS {
	case OSLinux:
		return []string{"tgz", "tgz.sig", p.Pkg.String()}
	case OSMac:
		return []string{"zip"}
	case OSWindows:
		return []string{"zip", "zip.sig", "msi"}
	}
	panic("unreachable")
}

func (p Platform) asGolangString() string {
	tmpl := `
{
    Name: "%s",
    Arch: %s,
    OS: %s,%s%s
    BuildTags: %s,%s
}`

	var pkg string
	if p.Pkg != "" {
		pkg = indentGolangField("Pkg", p.Pkg.ConstName())
	}

	var repos string
	if len(p.Repos) > 0 {
		var consts []string
		for _, r := range p.Repos {
			consts = append(consts, r.ConstName())
		}
		sort.Strings(consts)
		repos = indentGolangField("Repos", fmt.Sprintf("[]Repo{%s}", strings.Join(consts, ", ")))
	}

	var buildTags string
	if len(p.BuildTags) > 0 {
		if len(p.BuildTags) == len(defaultBuildTags) {
			buildTags = "defaultBuildTags"
		} else {
			var tags []string
			for _, t := range p.BuildTags {
				tags = append(tags, fmt.Sprintf(`"%s"`, t))
			}
			sort.Strings(tags)
			buildTags = fmt.Sprintf("[]string{%s}", strings.Join(tags, ", "))
		}
	}

	var binaryExt string
	if p.BinaryExt != "" {
		binaryExt = indentGolangField("BinaryExt", fmt.Sprintf(`"%s"`, p.BinaryExt))
	}

	return fmt.Sprintf(tmpl, p.Name, p.Arch.ConstName(), p.OS.ConstName(), pkg, repos, buildTags, binaryExt)
}

func indentGolangField(name, value string) string {
	return fmt.Sprintf("\n    %s: %s,", name, value)
}

func (o OS) ConstName() string {
	switch o {
	case OSWindows:
		return "OSWindows"
	case OSLinux:
		return "OSLinux"
	case OSMac:
		return "OSMac"
	}
	panic("unreachable")
}

func (o OS) String() string {
	return string(o)
}

func (p Pkg) ConstName() string {
	switch p {
	case PkgDeb:
		return "PkgDeb"
	case PkgRPM:
		return "PkgRPM"
	}
	panic("unreachable")
}

func (p Pkg) String() string {
	return string(p)
}

func (r Repo) ConstName() string {
	switch r {
	case RepoOrg:
		return "RepoOrg"
	case RepoEnterprise:
		return "RepoEnterprise"
	}
	panic("unreachable")
}

func (r Repo) String() string {
	return string(r)
}

func (a Arch) ConstName() string {
	switch a {
	case ArchArm64:
		return "ArchArm64"
	case ArchAarch64:
		return "ArchAarch64"
	case ArchS390x:
		return "ArchS390x"
	case ArchPpc64le:
		return "ArchPpc64le"
	case ArchX86_64:
		return "ArchX86_64"
	}
	panic("unreachable")
}

func (a Arch) String() string {
	return string(a)
}

var platformsByVariant map[string]Platform
var defaultBuildTags = []string{"ssl", "sasl", "gssapi", "failpoints"}

// Please keep this list sorted by Name and then Arch. This makes it easier to determine
// whether a given platform exists in the list.
var platforms = []Platform{
	{
		Name:      "amazon",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "amazon2",
		Arch:      ArchAarch64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "amazon2",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian10",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian11",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian81",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "debian92",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
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
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	// This is a special build that we upload to S3 but not to the release
	// repos.
	{
		Name: "rhel62",
		// This needs to match the name of the buildvariant in the Evergreen
		// config.
		VariantName:     "rhel62-no-sasl-or-kerberos",
		Arch:            ArchX86_64,
		OS:              OSLinux,
		Pkg:             PkgRPM,
		BuildTags:       []string{"ssl", "failpoints"},
		SkipForJSONFeed: true,
	},
	{
		Name:      "rhel70",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel71",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel72",
		Arch:      ArchS390x,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel80",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel81",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel82",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel83",
		Arch:      ArchS390x,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel90",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "suse12",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "suse15",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1604",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: []string{"failpoints", "ssl"},
	},
	{
		Name:      "ubuntu1604",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1804",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: []string{"failpoints", "ssl"},
	},
	{
		Name:      "ubuntu1804",
		Arch:      ArchPpc64le,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu1804",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu2004",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: []string{"failpoints", "ssl"},
	},
	{
		Name:      "ubuntu2004",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "ubuntu2204",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: []string{"failpoints", "ssl"},
	},
	{
		Name:      "ubuntu2204",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "windows",
		Arch:      ArchX86_64,
		OS:        OSWindows,
		BuildTags: defaultBuildTags,
		BinaryExt: ".exe",
	},
}

func Platforms() []Platform {
	return platforms
}
