package buildscript

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pkg/errors"
	"golang.org/x/mod/modfile"
)

func WriteSBOMLite(ctx *task.Context) error {
	if err := requirePodman(ctx); err != nil {
		return err
	}
	if err := startPodmanMachine(ctx); err != nil {
		return err
	}
	//nolint:errcheck
	defer stopPodmanMachine(ctx)

	return sh.Run(ctx, "scripts/regenerate-sbom-lite.sh")
}

// WriteAugmentedSBOM creates the SBOM Lite file for this project. This requires the following env
// vars to be set:
//
//   - KONDUKTO_TOKEN
//   - EVG_TRIGGERED_BY_TAG
func WriteAugmentedSBOM(ctx *task.Context) error {
	if err := requirePodman(ctx); err != nil {
		return err
	}

	return sh.Run(ctx, "scripts/regenerate-augmented-sbom.sh")
}

func requirePodman(ctx *task.Context) error {
	err := sh.Run(ctx, "which", "podman")
	if err == nil {
		return nil
	}

	fmt.Println(`This command requires the "podman" CLI tool, which you will need to install.`)
	fmt.Println("See https://podman.io/ for more information and installation instructions.")
	return err
}

func startPodmanMachine(ctx *task.Context) error {
	if runtime.GOOS == "linux" {
		// Linux doesn't need a podman machine to be up.
		return nil
	}

	out, err := sh.RunOutput(ctx, "podman", "machine", "info", "--format", "json")
	if err != nil {
		return err
	}

	fmt.Printf("podman machine info: %s\n", out)

	info := struct {
		Host struct {
			CurrentMachine string `json:"CurrentMachine"`
			MachineState   string `json:"MachineState"`
		} `json:"Host"`
	}{}
	err = json.Unmarshal([]byte(out), &info)
	if err != nil {
		return err
	}

	// Run podman machine init if there's no current machine.
	if info.Host.CurrentMachine == "" {
		err = sh.RunCmd(ctx, exec.CommandContext(ctx, "podman", "machine", "init"))
		if err != nil {
			return err
		}
	}

	if info.Host.MachineState == "Running" {
		return nil
	}

	return sh.Run(ctx, "podman", "machine", "start")
}

func stopPodmanMachine(ctx *task.Context) error {
	if runtime.GOOS == "linux" {
		// Linux doesn't need a podman machine.
		return nil
	}
	return sh.Run(ctx, "podman", "machine", "stop")
}

//nolint:misspell // "licence" is intentional here
var (
	// This matches a file that starts with "license" or "licence", in any
	// case, with an optional extension.
	licenseRegexp1 = regexp.MustCompile("(?i)^licen[cs]e(?:\\..+)?$")
	// This matches a file that has an extension of "license" or "licence", in
	// any case.
	licenseRegexp2      = regexp.MustCompile("(?i)\\.licen[cs]e$")
	trailingSpaceRegexp = regexp.MustCompile("(?m)[^\\n\\S]+$")

	horizontalLine = strings.Repeat("-", 70)
)

// WriteThirdPartyNotices writes the `THIRD-PARTY-NOTICES` file for this project, which contains all
// the licenses for our vendored code.
func WriteThirdPartyNotices(ctx *task.Context) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	licenseFiles, err := getLicenseFiles(root)
	if err != nil {
		return err
	}

	var notices string
	for _, lf := range licenseFiles {
		notices += "\n"
		notices += horizontalLine
		notices += "\n"
		notices += fmt.Sprintf(
			"License notice for %s (%s)\n",
			lf.packageName,
			filepath.Base(lf.path),
		)
		notices += horizontalLine
		notices += "\n"
		notices += "\n"

		content, err := os.ReadFile(lf.path)
		if err != nil {
			return err
		}

		contentStr := string(content)

		// Trim trailing space from each line.
		contentStr = trailingSpaceRegexp.ReplaceAllString(contentStr, "")

		notices += contentStr
	}

	return os.WriteFile(filepath.Join(root, "THIRD-PARTY-NOTICES"), []byte(notices), 0644)
}

const vendorDir string = "vendor"

type licenseFile struct {
	packageName string
	path        string
}

func getLicenseFiles(root string) ([]licenseFile, error) {
	var (
		walkIn       = filepath.Join(root, vendorDir)
		pathPrefix   = walkIn + "/"
		licenseFiles []licenseFile
	)
	err := filepath.WalkDir(
		walkIn,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !d.Type().IsRegular() {
				return nil
			}

			filename := d.Name()

			if licenseRegexp1.MatchString(filename) || licenseRegexp2.MatchString(filename) {
				packageName := strings.TrimPrefix(filepath.Dir(path), pathPrefix)
				licenseFiles = append(licenseFiles, licenseFile{packageName, path})
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	sort.Slice(
		licenseFiles,
		func(i, j int) bool {
			return licenseFiles[i].path < licenseFiles[j].path
		},
	)

	return licenseFiles, nil
}

// AddDep adds a new dependency. Pass a package name with an optional `@$version` at the end.
func AddDep(ctx *task.Context) error {
	return addOrUpdateGoDep(ctx, ctx.Get("pkg"), false)
}

// UpdateDep updates an existing dependency. Pass a package name with an optional `@$version` at the
// end.
func UpdateDep(ctx *task.Context) error {
	return addOrUpdateGoDep(ctx, ctx.Get("pkg"), true)
}

// UpdateAll updates all existing dependencies to their latest versions. To exclude one or more
// packages, set the `-exclude` argument to a list of packages separated by a space.
//
// This does not upgrade packages included as the replacement in a `replace` block. Those must be
// upgraded by editing the `go.mod` file directly.
func UpdateAllDeps(ctx *task.Context) error {
	pkgs, err := allGoDependencies()
	if err != nil {
		return err
	}

	excludeSet := mapset.NewSet(strings.Fields(ctx.Get("exclude"))...)

	for _, pkg := range pkgs {
		if excludeSet.Contains(pkg) {
			fmt.Printf("Excluding %s from the package updates\n", pkg)
			continue
		}
		if err := goGet(ctx, pkg, true); err != nil {
			return err
		}
	}

	return updateGoPackageMetadata(ctx)
}

func allGoDependencies() ([]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}

	goModPath := filepath.Join(root, "go.mod")
	raw, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read go.mod file at %s", goModPath)
	}

	file, err := modfile.Parse(goModPath, raw, nil)
	if err != nil {
		return nil, err
	}

	modules := mapset.NewSet[string]()
	for _, req := range file.Require {
		modules.Add(req.Mod.Path)
	}

	return mapset.Sorted(modules), nil
}

func addOrUpdateGoDep(ctx *task.Context, pkg string, isUpdate bool) error {
	if err := goGet(ctx, pkg, isUpdate); err != nil {
		return err
	}
	return updateGoPackageMetadata(ctx)
}

func goGet(ctx *task.Context, pkg string, isUpdate bool) error {
	v, err := goVersion(ctx)
	if err != nil {
		return err
	}

	args := []string{"get"}
	if isUpdate {
		args = append(args, "-u")
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	// Setting GOTOOLCHAIN to the current version prevents Go from trying to update itself because a
	// dependency we add or update requires a newer Go version.
	//
	// If we set _anything_ in `cmd.Env` then we don't get any of our current env vars, so we pass
	// the current env through and then overwrite GOTOOLCHAIN.
	cmd.Env = append(
		syscall.Environ(),
		"GOTOOLCHAIN="+v,
	)

	return sh.RunCmd(ctx, cmd)
}

var versionRE = regexp.MustCompile(`go version (go\d+\.\d+\.\d+) `)
var v string

func goVersion(ctx *task.Context) (string, error) {
	if v != "" {
		return v, nil
	}
	out, err := sh.RunOutput(ctx, "go", "version")
	if err != nil {
		return "", err
	}

	matches := versionRE.FindStringSubmatch(out)
	if len(matches) < 1 {
		return "", fmt.Errorf(
			"could not parse go version from `go version` output: %s",
			strings.TrimSpace(out),
		)
	}

	v = matches[1]

	return v, nil
}

func updateGoPackageMetadata(ctx *task.Context) error {
	if err := sh.Run(ctx, "go", "mod", "tidy"); err != nil {
		return err
	}
	if err := sh.Run(ctx, "go", "mod", "vendor"); err != nil {
		return err
	}
	if err := WriteSBOMLite(ctx); err != nil {
		return err
	}

	return WriteThirdPartyNotices(ctx)
}
