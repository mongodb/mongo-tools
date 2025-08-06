package platform

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/mongodb/mongo-tools/release/env"
	"github.com/mongodb/mongo-tools/release/version"
	"github.com/samber/lo"
)

type OS string

const (
	OSWindows OS = "windows"
	OSLinux   OS = "linux"
	OSMac     OS = "mac"
)

func (o OS) GoOS() string {
	switch o {
	case OSMac:
		return "darwin"
	default:
		return string(o)
	}
}

type Pkg string

const (
	PkgDeb Pkg = "deb"
	PkgRPM Pkg = "rpm"
)

type Repo string

const (
	RepoOrg        Repo = "org"
	RepoEnterprise Repo = "enterprise"
)

type Arch string

const (
	ArchArm64 Arch = "arm64"
	// While arm64 and aarch64 are the same architecture, some Linux distros use arm64 and others use aarch64:
	// - aarch64: RHEL/Amazon/SUSE
	// - arm64: Debian/Ubuntu.
	ArchAarch64 Arch = "aarch64"
	ArchS390x   Arch = "s390x"
	ArchPpc64le Arch = "ppc64le"
	ArchX86_64  Arch = "x86_64"
)

// GoArch returns the GOARCH value for a given architecture.
func (a Arch) GoArch() string {
	switch a {
	case ArchAarch64:
		return "arm64"
	case ArchX86_64:
		return "amd64"
	default:
		return string(a)
	}
}

// Platform represents a platform (a combination of OS, distro,
// version, and architecture) on which we may build/test the tools.
// There should be at least one evergreen buildvariant per platform,
// and there may be multiple.
type Platform struct {
	Name string
	// This is used to override the variant name. It should only be used for
	// special builds. In general, we want to use the OS name + arch for the
	// variant name.
	VariantName        string
	Arch               Arch
	OS                 OS
	Pkg                Pkg
	Repos              []Repo
	BuildTags          []string
	SkipForJSONFeed    bool
	ServerVariantNames mapset.Set[string]
	ServerPlatform     string
	// If set, this a linux release will only be pushed to server repos within this range (inclusive).
	MinLinuxServerVersion *version.Version
	MaxLinuxServerVersion *version.Version
}

func (p Platform) Variant() string {
	if p.VariantName != "" {
		return p.VariantName
	}

	return createVariantName(p.Name, p.Arch)
}

// GetFromEnv returns the Platform for this host, based on the value
// of EVG_VARIANT. It returns an error if EVG_VARIANT is unset or set
// to an unknown value.
func GetFromEnv() (Platform, error) {
	variant, err := env.EvgVariant()
	if err != nil {
		return Platform{}, err
	}

	if variant == "rhel88-race" {
		variant = "rhel88"
	}

	pf, ok := GetByVariant(variant)
	if !ok {
		return Platform{}, fmt.Errorf("unknown evg variant %q", variant)
	}
	return pf, nil
}

// DetectLocal detects the platform for non-evergreen use cases.
func DetectLocal() (Platform, error) {

	cmd := exec.Command("uname", "-sm")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return Platform{}, fmt.Errorf("failed to run uname: %w", err)
	}

	kernelNameAndArch := strings.TrimSpace(string(out))
	pieces := regexp.MustCompile(`[ \t]+`).Split(kernelNameAndArch, -1)
	if len(pieces) != 2 {
		panic(fmt.Sprintf("Unexpected uname output (%d pieces): %q", len(pieces), string(out)))
	}

	kernelName := pieces[0]
	archName := Arch(pieces[1])

	var os string
	var pf Platform
	var foundPf bool

	if strings.HasPrefix(kernelName, "CYGWIN") || strings.HasPrefix(kernelName, "MSYS_NT") {
		os = "windows"
		pf, foundPf = GetByVariant("windows")
	} else {
		switch kernelName {
		case "Linux":
			var version string

			os, version, err = GetLinuxDistroAndVersion()
			if err != nil {
				return Platform{}, fmt.Errorf(
					"detecting local Linux distro/version: %w",
					err,
				)
			}

			os = strings.ToLower(os)
			version = strings.ReplaceAll(version, ".", "")

			os += version
		case "Darwin":
			os = "macos"
		default:
			return Platform{}, fmt.Errorf("failed to detect local platform from kernel name %q", kernelName)
		}

		pf, foundPf = GetByOsAndArch(os, archName)
	}

	if !foundPf {
		return Platform{}, fmt.Errorf(
			"no platform %s/%s found; did %s’s platform name change?",
			os,
			archName,
			os,
		)
	}

	return pf, nil
}

func GetLinuxDistroAndVersion() (string, string, error) {
	cmd := exec.Command("lsb_release", "--short", "--id")
	cmd.Stderr = os.Stderr
	distro, err := cmd.Output()

	if err != nil {
		return "", "", fmt.Errorf("fetching Linux distro name: %w", err)
	}

	cmd = exec.Command("lsb_release", "--short", "--release")
	cmd.Stderr = os.Stderr
	version, err := cmd.Output()

	if err != nil {
		return "", "", fmt.Errorf("fetching %#q version: %w", distro, err)
	}

	distroStr := string(bytes.TrimSpace(distro))
	versionStr := string(bytes.TrimSpace(version))

	return distroStr, versionStr, nil
}

func GetByVariant(variant string) (Platform, bool) {
	if platformsByVariant == nil {
		platformsByVariant = make(map[string]Platform)
		for _, p := range platforms {
			if _, exists := platformsByVariant[p.Variant()]; exists {
				panic("Duplicate variant: " + p.Variant())
			}

			platformsByVariant[p.Variant()] = p
		}
	}
	p, ok := platformsByVariant[variant]
	return p, ok
}

func GetByOsAndArch(os string, arch Arch) (Platform, bool) {
	return GetByVariant(createVariantName(os, arch))
}

func createVariantName(os string, arch Arch) string {
	if arch == ArchX86_64 {
		return os
	}
	return os + "-" + string(arch)
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
		return []string{"tgz", "tgz.sig", p.Pkg.String(), p.Pkg.String() + ".sig"}
	case OSMac:
		return []string{"zip"}
	case OSWindows:
		return []string{"zip", "zip.sig", "msi"}
	}
	panic(fmt.Sprintf("unreachable; os=%#q", p.OS))
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

	binaryExt := GetLocalBinaryExt()
	if binaryExt != "" {
		binaryExt = indentGolangField("BinaryExt", fmt.Sprintf(`"%s"`, binaryExt))
	}

	return fmt.Sprintf(
		tmpl,
		p.Name,
		p.Arch.ConstName(),
		p.OS.ConstName(),
		pkg,
		repos,
		buildTags,
		binaryExt,
	)
}

func GetLocalBinaryExt() string {
	return lo.Ternary(
		runtime.GOOS == "windows",
		".exe",
		"",
	)
}

var canonicalTarget = map[string]string{
	"rhel80": "rhel8",
}

func (p Platform) TargetMatches(target string) bool {
	baseTarget := cmp.Or(p.ServerPlatform, p.Name)

	for _, ref := range []*string{&target, &baseTarget} {
		if canonical, has := canonicalTarget[*ref]; has {
			*ref = canonical
		}
	}

	return target == baseTarget
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
	panic(fmt.Sprintf("unreachable; os=%#q", o))
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
	panic(fmt.Sprintf("unreachable; pkg=%#q", p))
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
	panic(fmt.Sprintf("unreachable; repo=%#q", r))
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
	panic(fmt.Sprintf("unreachable; arch=%#q", a))
}

func (a Arch) String() string {
	return string(a)
}

var platformsByVariant map[string]Platform
var defaultBuildTags = []string{"gssapi", "failpoints"}

// Please keep this list sorted by Name and then Arch. This makes it easier to determine
// whether a given platform exists in the list.
var platforms = []Platform{
	{
		Name:                  "amazon",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "amazon2",
		Arch:                  ArchAarch64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "amazon2",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:      "amazon2023",
		Arch:      ArchAarch64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "amazon2023",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
	},
	{
		Name:                  "debian10",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "debian11",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:               "debian12",
		Arch:               ArchX86_64,
		OS:                 OSLinux,
		Pkg:                PkgDeb,
		Repos:              []Repo{RepoEnterprise, RepoOrg},
		BuildTags:          defaultBuildTags,
		ServerVariantNames: mapset.NewSet("enterprise-debian12-64"),
	},
	{
		Name:                  "debian92",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:               "macos",
		Arch:               ArchArm64,
		OS:                 OSMac,
		BuildTags:          defaultBuildTags,
		ServerVariantNames: mapset.NewSet("enterprise-macos-arm64"),
	},
	{
		Name:               "macos",
		Arch:               ArchX86_64,
		OS:                 OSMac,
		BuildTags:          defaultBuildTags,
		ServerVariantNames: mapset.NewSet("enterprise-macos"),
	},
	{
		// mongodump_passthru_v is the evergreen variant name used for
		// the passthrough tests. It currently maps to an amazon2-aarch64,
		// but having a distinct variant name helps in managing Evergreen
		// and Build Baron.
		Name:                  "mongodump_passthru_v",
		VariantName:           "mongodump_passthru_v",
		Arch:                  ArchAarch64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "rhel70",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "rhel71",
		Arch:                  ArchPpc64le,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "rhel72",
		Arch:                  ArchS390x,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		// Same variant name as mongosync passthrough tests to minimize
		// changes in mongodump-task-gen for mongodump passthrough tests.
		Name:            "rhel80",
		Arch:            ArchX86_64,
		OS:              OSLinux,
		Pkg:             PkgRPM,
		Repos:           []Repo{RepoOrg, RepoEnterprise},
		BuildTags:       defaultBuildTags,
		SkipForJSONFeed: true,
		// Using server rhel 80 builds because "enterprise-rhel-80-64-bit" is not available for all server versions.
		// NB: Older builds are “rhel-80”, while newer ones are just “rhel-8”.
		ServerVariantNames: mapset.NewSet(
			"enterprise-rhel-80-64-bit",
			"enterprise-rhel-8-64-bit",
		),
		ServerPlatform: "rhel80",
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
		Name:      "rhel83",
		Arch:      ArchS390x,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel88",
		Arch:      ArchAarch64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
	},
	{
		Name:      "rhel88",
		Arch:      ArchX86_64,
		OS:        OSLinux,
		Pkg:       PkgRPM,
		Repos:     []Repo{RepoOrg, RepoEnterprise},
		BuildTags: defaultBuildTags,
		// Using server rhel 80 builds because "enterprise-rhel-80-64-bit" is not available for all server versions.
		// NB: Older builds are “rhel-80”, while newer ones are just “rhel-8”.
		ServerVariantNames: mapset.NewSet(
			"enterprise-rhel-80-64-bit",
			"enterprise-rhel-8-64-bit",
		),
		ServerPlatform: "rhel80",
	},
	// MongoDB server only supports enterprise on RHEL9 for s390x and ppc64le, and only version 7.0+ is available.
	{
		Name:                  "rhel9",
		Arch:                  ArchPpc64le,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise},
		BuildTags:             defaultBuildTags,
		MinLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "rhel9",
		Arch:                  ArchS390x,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise},
		BuildTags:             defaultBuildTags,
		MinLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "rhel93",
		Arch:                  ArchAarch64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoOrg, RepoEnterprise},
		BuildTags:             defaultBuildTags,
		MinLinuxServerVersion: &version.Version{Major: 6, Minor: 0, Patch: 0},
	},
	{
		Name:                  "rhel93",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoOrg, RepoEnterprise},
		BuildTags:             defaultBuildTags,
		MinLinuxServerVersion: &version.Version{Major: 6, Minor: 0, Patch: 0},
	},
	{
		Name:                  "suse12",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgRPM,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
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
		Name:                  "ubuntu1604",
		Arch:                  ArchArm64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "ubuntu1604",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "ubuntu1804",
		Arch:                  ArchArm64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:                  "ubuntu1804",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MaxLinuxServerVersion: &version.Version{Major: 7, Minor: 0, Patch: 0},
	},
	{
		Name:      "ubuntu2004",
		Arch:      ArchArm64,
		OS:        OSLinux,
		Pkg:       PkgDeb,
		Repos:     []Repo{RepoEnterprise, RepoOrg},
		BuildTags: defaultBuildTags,
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
		BuildTags: defaultBuildTags,
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
		Name:                  "ubuntu2404",
		Arch:                  ArchArm64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MinLinuxServerVersion: &version.Version{Major: 8, Minor: 0, Patch: 0},
	},
	{
		Name:                  "ubuntu2404",
		Arch:                  ArchX86_64,
		OS:                    OSLinux,
		Pkg:                   PkgDeb,
		Repos:                 []Repo{RepoEnterprise, RepoOrg},
		BuildTags:             defaultBuildTags,
		MinLinuxServerVersion: &version.Version{Major: 8, Minor: 0, Patch: 0},
	},
	{
		Name:               "windows",
		Arch:               ArchX86_64,
		OS:                 OSWindows,
		BuildTags:          defaultBuildTags,
		ServerVariantNames: mapset.NewSet("enterprise-windows"),
	},
}

func Platforms() []Platform {
	return platforms
}
