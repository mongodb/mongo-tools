package buildscript

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/craiggwilson/goke/pkg/git"
	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
	"github.com/mongodb/mongo-tools-common/testtype"
	"github.com/mongodb/mongo-tools/release/platform"
)

// toolNames is a list of the names of all the tools.
var toolNames = []string{
	"bsondump",
	"mongodump", "mongorestore",
	"mongoimport", "mongoexport",
	"mongostat", "mongotop",
	"mongofiles",
}

// BuildTools is an Executor that builds the tools.
func BuildTools(ctx *task.Context) error {
	for _, tool := range selectedTools(ctx) {
		err := buildToolBinary(ctx, tool, "bin")
		if err != nil {
			return err
		}
	}
	return nil
}

// TestUnit is an Executor that runs the tools unit tests.
func TestUnit(ctx *task.Context) error {
	for _, tool := range selectedTools(ctx) {
		err := runToolTests(ctx, tool, testtype.UnitTestType)
		if err != nil {
			return err
		}
	}
	return nil
}

// TestKerberos is an Executor that runs the tools kerberos tests.
func TestKerberos(ctx *task.Context) error {
	for _, tool := range selectedTools(ctx) {
		err := runToolTests(ctx, tool, testtype.KerberosTestType)
		if err != nil {
			return err
		}
	}
	return nil
}

// TestIntegration is an Executor that runs the tools integration
// tests.
func TestIntegration(ctx *task.Context) error {
	for _, tool := range selectedTools(ctx) {
		err := runToolTests(ctx, tool, testtype.IntegrationTestType)
		if err != nil {
			return err
		}
	}
	return nil
}

// buildToolBinary builds the tool with the specified name, putting
// the resulting binary into outDir.
func buildToolBinary(ctx *task.Context, tool string, outDir string) error {
	pf, err := platform.GetFromEnv()
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

// runToolTests runs the tests of the provided testType for the tool
// with the specified name.
func runToolTests(ctx *task.Context, tool string, testType string) error {
	outFile, err := sh.CreateFileR(ctx, fmt.Sprintf("testing_output/%s.suite", tool))
	if err != nil {
		return fmt.Errorf("failed to create testing output file: %w", err)
	}
	defer outFile.Close()

	buildFlags, err := getBuildFlags(ctx)
	if err != nil {
		return fmt.Errorf("failed to get build flags: %w", err)
	}

	args := []string{"test"}
	args = append(args, buildFlags...)
	if ctx.Verbose {
		args = append(args, "-v")
	}

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
	cmd.Dir = tool
	cmd.Env = env

	err = sh.RunCmd(ctx, cmd)
	if err != nil {
		return err
	}

	return nil
}

// getTags gets the go build tags that should be used for the current
// platform.
func getTags(ctx *task.Context) ([]string, error) {
	pf, err := platform.GetFromEnv()
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

	pf, err := platform.GetFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to get platform: %w", err)
	}

	if pf.OS == platform.OSLinux {
		flags = append(flags, "-buildmode=pie")
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

// selectedTools gets the list of tools selected via the -tools flag,
// defaulting to the list of all tools.
func selectedTools(ctx *task.Context) []string {
	selectedTools := toolNames
	if tools := ctx.Get("tools"); tools != "" {
		selectedTools = strings.Split(tools, ",")
	}
	return selectedTools
}
