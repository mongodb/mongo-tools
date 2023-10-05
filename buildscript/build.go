package buildscript

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/craiggwilson/goke/pkg/git"
	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/release/platform"
)

// pkgNames is a list of the names of all the packages to test or build.
var pkgNames = []string{
	"bsondump",
	"mongodump", "mongorestore",
	"mongoimport", "mongoexport",
	"mongostat", "mongotop",
	"mongofiles",
	"common",
	"release",
}

// minimumGoVersion must be prefixed with v to be parsed by golang.org/x/mod/semver
var minimumGoVersion = "v1.20.0"

func CheckMinimumGoVersion(ctx *task.Context) error {
	goVersionStr, err := runCmd(ctx, "go", "version")
	if err != nil {
		return fmt.Errorf("failed to get current go version: %w", err)
	}

	_, _ = ctx.Write([]byte(fmt.Sprintf("Found Go version \"%s\"\n", goVersionStr)))

	versionPattern := `go(\d+\.\d+\.*\d*)`

	r := regexp.MustCompile(versionPattern)
	goVersionMatches := r.FindStringSubmatch(goVersionStr)
	if len(goVersionMatches) < 2 {
		return fmt.Errorf("Could not find version string in the output of `go version`. Output: %s", goVersionStr)
	}

	// goVersion must be prefixed with v to be parsed by golang.org/x/mod/semver
	goVersion := fmt.Sprintf("v%s", goVersionMatches[1])

	if semver.Compare(goVersion, minimumGoVersion) < 0 {
		return fmt.Errorf("Could not find minimum desired Go version. Found %s, Wanted at least %s", goVersion, minimumGoVersion)
	}

	return nil
}

// BuildTools is an Executor that builds the tools.
func BuildTools(ctx *task.Context) error {
	for _, pkg := range selectedPkgs(ctx) {
		if pkg != "common" && pkg != "release" {
			err := buildToolBinary(ctx, pkg, "bin")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// TestUnit is an Executor that runs all unit tests for the provided packages.
func TestUnit(ctx *task.Context) error {
	return runTests(ctx, selectedPkgs(ctx), testtype.UnitTestType)
}

// TestKerberos is an Executor that runs all kerberos tests for the provided packages.
func TestKerberos(ctx *task.Context) error {
	return runTests(ctx, selectedPkgs(ctx), testtype.KerberosTestType)
}

// TestIntegration is an Executor that runs all integration tests for the provided packages.
func TestIntegration(ctx *task.Context) error {
	return runTests(ctx, selectedPkgs(ctx), testtype.IntegrationTestType)
}

// TestSRV is an Executor that runs all SRV tests for the provided packages.
func TestSRV(ctx *task.Context) error {
	return runTests(ctx, selectedPkgs(ctx), testtype.SRVConnectionStringTestType)
}

// TestAWSAuth is an Executor that runs all AWS auth tests for the provided packages.
func TestAWSAuth(ctx *task.Context) error {
	return runTests(ctx, selectedPkgs(ctx), testtype.AWSAuthTestType)
}

// buildToolBinary builds the tool with the specified name, putting
// the resulting binary into outDir.
func buildToolBinary(ctx *task.Context, tool string, outDir string) error {
	pf, err := getPlatform()
	if err != nil {
		return err
	}

	outPath := filepath.Join(outDir, tool+pf.BinaryExt)
	_ = sh.Remove(ctx, outPath)

	mainFile := filepath.Join(tool, "main", fmt.Sprintf("%s.go", tool))

	buildFlags, err := getBuildFlags(ctx)
	if err != nil {
		return fmt.Errorf("failed to get build flags: %w", err)
	}

	args := []string{
		"build",
		"-o", outPath,
	}
	args = append(args, buildFlags...)
	args = append(args, mainFile)

	cmd := exec.CommandContext(ctx, "go", args...)
	sh.LogCmd(ctx, cmd)
	output, err := cmd.CombinedOutput()

	if len(output) > 0 {
		_, _ = ctx.Write(output)
	}

	if err != nil {
		return fmt.Errorf("failed to build %s: %w", tool, err)
	}
	return nil
}

// runTests runs the tests of the provided testType for the provided packages.
func runTests(ctx *task.Context, pkgs []string, testType string) error {
	for _, pkg := range pkgs {
		outFile, err := sh.CreateFileR(ctx, fmt.Sprintf("testing_output/%s.suite", pkg))
		if err != nil {
			return fmt.Errorf("failed to create testing output file: %w", err)
		}
		defer outFile.Close()

		buildFlags, err := getBuildFlags(ctx)
		if err != nil {
			return fmt.Errorf("failed to get build flags: %w", err)
		}

		// Use the recursive wildcard (...) to run all tests
		// of the provided testType for the current pkg.
		args := []string{"test", "./" + pkg + "/..."}
		args = append(args, buildFlags...)
		if ctx.Verbose {
			args = append(args, "-v")
		}

		// Append any existing environment variables, along
		// with the ones indicating which test types to run.
		env := append([]string{}, os.Environ()...)
		env = append(env, testType+"=true")
		if ctx.Get("ssl") == "true" {
			env = append(env, testtype.SSLTestType+"=true")
		}
		if ctx.Get("auth") == "true" {
			env = append(env, testtype.AuthTestType+"=true")
		}

		out := io.MultiWriter(ctx, outFile)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.Env = env

		err = sh.RunCmd(ctx, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

// getTags gets the go build tags that should be used for the current
// platform.
func getTags(ctx *task.Context) ([]string, error) {
	pf, err := getPlatform()
	if err != nil {
		return nil, err
	}
	return pf.BuildTags, nil
}

// getLdflags gets the ldflags that should be used when building the
// tools on the current platform.
func getLdflags(ctx *task.Context) (string, error) {
	versionStr, err := runCmd(ctx, "go", "run", "release/release.go", "get-version")
	if err != nil {
		return "", fmt.Errorf("failed to get current version: %w", err)
	}

	gitCommit, err := git.SHA1(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get git commit hash: %w", err)
	}

	ldflags := fmt.Sprintf("-X main.VersionStr=%s -X main.GitCommit=%s", versionStr, gitCommit)
	return ldflags, nil
}

// getBuildFlags gets all the build flags that should be used when
// building the tools on the current platform, including tags and ldflags.
func getBuildFlags(ctx *task.Context) ([]string, error) {
	ldflags, err := getLdflags(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ldflags: %w", err)
	}

	tags, err := getTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}

	flags := []string{
		"-ldflags", ldflags,
		"-tags", strings.Join(tags, " "),
	}

	pf, err := getPlatform()
	if err != nil {
		return nil, fmt.Errorf("failed to get platform: %w", err)
	}

	if pf.OS == platform.OSLinux {
		flags = append(flags, "-buildmode=pie")
	} else if pf.OS == platform.OSWindows {
		flags = append(flags, "-buildmode=exe")
	}

	return flags, nil
}

// runCmd runs the command with the provided name and arguments, and
// returns the command's output as a trimmed string.
func runCmd(ctx *task.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	sh.LogCmd(ctx, cmd)
	output, err := cmd.CombinedOutput()
	return string(bytes.TrimSpace(output)), err
}

// selectedPkgs gets the list of packages selected via the -pkgs flag,
// defaulting to the list of all packages.
func selectedPkgs(ctx *task.Context) []string {
	selectedPkgs := pkgNames
	if pkgs := ctx.Get("pkgs"); pkgs != "" {
		selectedPkgs = strings.Split(pkgs, ",")
	}
	return selectedPkgs
}

func getPlatform() (platform.Platform, error) {
	if os.Getenv("CI") != "" {
		return platform.GetFromEnv()
	}
	return platform.DetectLocal()
}
