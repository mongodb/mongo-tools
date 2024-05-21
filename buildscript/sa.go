package buildscript

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
)

const (
	gosecPkg = "github.com/securego/gosec/v2/cmd/gosec@v2.20.0"

	preciousVersion = "0.7.2"
	ubiVersion      = "0.0.18"
)

func SAInstallDevTools(ctx *task.Context) error {
	if err := installGosec(ctx); err != nil {
		return err
	}
	if err := installUBI(ctx); err != nil {
		return err
	}
	return installBinaryTool(
		ctx,
		"precious",
		preciousVersion,
		"houseabsolute/precious",
		fmt.Sprintf(
			"https://github.com/houseabsolute/precious/releases/download/v%s/precious-Linux-x86_64-musl.tar.gz",
			preciousVersion,
		),
	)
}

// Install UBI.
func installUBI(ctx *task.Context) error {
	var err error
	devBin, err := devBinDir()
	if err != nil {
		return err
	}

	ubi, err := devBinFile("ubi")
	if err != nil {
		return err
	}

	exists, err := executableExistsWithVersion(ctx, ubi, ubiVersion)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	var ubiBootstrapURL string
	switch runtime.GOOS {
	case "windows":
		ubiBootstrapURL = "https://raw.githubusercontent.com/houseabsolute/ubi/ci-for-bootstrap/bootstrap/bootstrap-ubi.ps1"
	default:
		ubiBootstrapURL = "https://raw.githubusercontent.com/houseabsolute/ubi/master/bootstrap/bootstrap-ubi.sh"
	}

	s := strings.Split(ubiBootstrapURL, "/")
	bootstrapPath := filepath.Join(os.TempDir(), s[len(s)-1])

	out, err := os.Create(bootstrapPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := httpGetWithRetries(ubiBootstrapURL, 5)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	var cmd []string
	if strings.HasSuffix(ubiBootstrapURL, ".ps1") {
		cmd = []string{"powershell", bootstrapPath}
	} else {
		cmd = []string{"sh", bootstrapPath}
	}

	// On Windows the bootstrapper always installs into the current directory,
	// so chdir there first.
	return doInDir(devBin, func() error {
		c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		c.Env = []string{"TARGET=" + devBin, "TAG=v" + ubiVersion}
		return sh.RunCmd(ctx, c)
	})
}

// Install gosec.
func installGosec(ctx *task.Context) error {
	return goInstall(ctx, gosecPkg)
}

// Install a Golang package as an executable with "go install".
func goInstall(ctx *task.Context, link string) error {
	return withRetries(
		5,
		fmt.Sprintf("go install %s", link),
		func() error {
			return sh.Run(ctx, "go", "install", link)
		},
	)
}

func installBinaryTool(ctx *task.Context, exeName, toolVersion, githubProject, downloadURLForCI string) error {
	devBin, err := devBinDir()
	if err != nil {
		return err
	}

	devBinExe, err := devBinFile(exeName)
	if err != nil {
		return err
	}

	exists, err := executableExistsWithVersion(ctx, devBinExe, preciousVersion)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	cmd := []string{
		filepath.Join(devBin, "ubi"),
		"--in", devBin,
	}
	if inCI() {
		// Using the `--url` arg avoids hitting the GitHub API, but it skips
		// all the platform detection ubi provides. We do it this way because
		// even with authentication, the limits on the GitHub API are
		// something like 5,000 requests an hour. Without it, the limit is way
		// lower.
		//
		// This seemed simpler than adding a GitHub token to Evergreen. If we
		// ever switch to GH Actions we can reconsider, since in that case
		// we'd have a token automatically available in the `GITHUB_TOKEN` env
		// var.
		cmd = append(cmd, "--url", downloadURLForCI)
	} else {
		cmd = append(
			cmd,
			"--project", githubProject,
			"--tag", "v"+toolVersion,
		)
	}

	return withRetries(
		5,
		fmt.Sprintf("installing %s", exeName),
		func() error {
			return sh.Run(ctx, cmd[0], cmd[1:]...)
		},
	)
}

func SAPreciousLint(ctx *task.Context) error {
	return runPrecious(ctx, "lint", "--all")
}

func runPrecious(ctx *task.Context, args ...string) error {
	devBin, err := devBinDir()
	if err != nil {
		return err
	}

	cmd := append(
		[]string{filepath.Join(devBin, "precious")},
		args...,
	)

	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return sh.RunCmd(ctx, c)
}

// SAModTidy runs go mod tidy and ensure no changes were made.
// Copied from mongohouse: https://github.com/10gen/mongohouse/blob/333308814f96a0909c8125f71af7748b263e3263/buildscript/sa.go#L72
func SAModTidy(ctx *task.Context) error {
	// Save original contents in case they get modified. When
	// https://github.com/golang/go/issues/27005 is done, we
	// shouldn't need this anymore.
	origGoMod, err := ioutil.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}
	origGoSum, err := ioutil.ReadFile("go.sum")
	if err != nil {
		return fmt.Errorf("error reading go.sum: %w", err)
	}

	err = sh.Run(ctx, "go", "mod", "tidy")
	if err != nil {
		return err
	}

	newGoMod, err := ioutil.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}
	newGoSum, err := ioutil.ReadFile("go.sum")
	if err != nil {
		return fmt.Errorf("error reading go.sum: %w", err)
	}

	if !bytes.Equal(origGoMod, newGoMod) || !bytes.Equal(origGoSum, newGoSum) {
		// Restore originals, ignoring errors since they need tidying anyway.
		_ = ioutil.WriteFile("go.mod", origGoMod, 0600)
		_ = ioutil.WriteFile("go.sum", origGoSum, 0600)
		return errors.New("go.mod and/or go.sum needs changes: run `go mod tidy` and commit the changes")
	}

	return nil
}

// SAEvergreenValidate runs `evergreen validate` on common.yml and ensures the file is valid.
func SAEvergreenValidate(ctx *task.Context) error {
	output, err := sh.RunOutput(ctx, "evergreen", "validate", "--file", "common.yml", "-p", "mongo-tools")
	if err != nil {
		return fmt.Errorf("error from `evergreen validate`: %s: %w", output, err)
	}

	// TODO: change this if-block in TOOLS-2840.
	// This check ignores any YAML warnings related to duplicate keys in YAML maps.
	// See ticket for more details.
	if strings.HasSuffix(output, "is valid with warnings") {
		for _, line := range strings.Split(output, "\n") {
			if !(strings.HasSuffix(line, "unmarshal errors:") ||
				strings.HasSuffix(line, "already set in map") ||
				strings.HasSuffix(line, "is valid with warnings")) {
				return fmt.Errorf("error from `evergreen validate`: %s", output)
			}
		}
	}

	return nil
}
