package buildscript

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/craiggwilson/goke/pkg/git"
	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/release/platform"
	"golang.org/x/mod/semver"
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

// minimumGoVersion must be prefixed with v to be parsed by golang.org/x/mod/semver.
const minimumGoVersion = "v1.22.3"

func CheckMinimumGoVersion(ctx *task.Context) error {
	goVersionStr, err := runCmd(ctx, "go", "version")
	if err != nil {
		return fmt.Errorf("failed to get current go version: %w", err)
	}

	_, _ = fmt.Fprintf(ctx, "Found Go version \"%s\"\n", goVersionStr)

	versionPattern := `go(\d+\.\d+\.*\d*)`

	r := regexp.MustCompile(versionPattern)
	goVersionMatches := r.FindStringSubmatch(goVersionStr)
	if len(goVersionMatches) < 2 {
		return fmt.Errorf(
			"Could not find version string in the output of `go version`. Output: %s",
			goVersionStr,
		)
	}

	// goVersion must be prefixed with v to be parsed by golang.org/x/mod/semver
	goVersion := fmt.Sprintf("v%s", goVersionMatches[1])

	if semver.Compare(goVersion, minimumGoVersion) < 0 {
		return fmt.Errorf(
			"Could not find minimum desired Go version. Found %s, Wanted at least %s",
			goVersion,
			minimumGoVersion,
		)
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

// TestAWSAuth is an Executor that runs all AWS auth tests for the provided packages.
func TestAWSAuth(ctx *task.Context) error {
	return runTests(ctx, selectedPkgs(ctx), testtype.AWSAuthTestType)
}

// buildToolBinary builds the tool with the specified name, putting
// the resulting binary into outDir.
func buildToolBinary(ctx *task.Context, tool string, outDir string) error {
	outPath := filepath.Join(outDir, tool+platform.GetLocalBinaryExt())
	_ = sh.Remove(ctx, outPath)

	mainFile := filepath.Join(tool, "main", fmt.Sprintf("%s.go", tool))

	buildFlags := getBuildFlags(ctx, false)

	args := []string{
		"build",
		"-o", outPath,
	}
	args = append(args, buildFlags...)
	args = append(args, mainFile)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stderr = os.Stderr
	sh.LogCmd(ctx, cmd)
	output, err := cmd.Output()

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

		buildFlags := getBuildFlags(ctx, true)

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
		if ctx.Get("topology") == "replSet" {
			env = append(env, testtype.ReplSetTestType+"=true")
		}

		if ctx.Get("race") == "true" {
			args = append(args, "-race")
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
func getTags(_ *task.Context) ([]string, error) {
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
func getBuildFlags(ctx *task.Context, forTests bool) []string {
	flags := []string{}

	ldflags, err := getLdflags(ctx)
	if err == nil {
		flags = append(flags, "-ldflags", ldflags)
	} else {
		ctx.Logf("failed to get ldflags (error: %v); will still attempt build\n", err)
	}

	tags, err := getTags(ctx)
	if err == nil {
		flags = append(flags, "-tags", strings.Join(tags, " "))
	} else {
		ctx.Logf("failed to get tags (error: %v); will still attempt build\n", err)
	}

	pf, err := getPlatform()
	if err == nil {
		switch pf.OS {
		case platform.OSLinux:
			// We don't want to enable -buildmode=pie for tests. This interferes with enabling the race
			// detector.
			if !forTests {
				flags = append(flags, "-buildmode=pie")
			}
		case platform.OSWindows:
			flags = append(flags, "-buildmode=exe")
		}
	} else {
		ctx.Logf("failed to get platform (error: %v); will still attempt build\n", err)
	}

	return flags
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

func getPlatform() (pf platform.Platform, err error) {
	if os.Getenv("CI") != "" {
		pf, err = platform.GetFromEnv()
		if err == nil {
			log.Printf("Platform from env: %+v\n", pf)
		}
	} else {
		pf, err = platform.DetectLocal()
		if err == nil {
			log.Printf("Platform detected: %+v\n", pf)
		}
	}

	return
}
