package buildscript

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
	"github.com/pkg/errors"
)

func devBinDir() (string, error) {
	return getRepoRootSubdir("dev-bin")
}

func devBinFile(filename string) (string, error) {
	return getDirExeName("dev-bin", filename)
}

// getDirExeName returns the full path to the file in a subdirectory of the repo
// root. For example: getDirExeName("dev-bin", "precious") returns
// /path/to/mongosync/dev-bin/precious on unix-like systems and
// C:\path\to\mongosync\dev-bin\precious.exe on Windows.
func getDirExeName(dirname, filename string) (string, error) {
	dir, err := getRepoRootSubdir(dirname)
	if err != nil {
		return "", err
	}

	target := filepath.Join(dir, filename)
	if runtime.GOOS == "windows" {
		target += ".exe"
	}

	return target, nil
}

// getRepoRootSubdir returns the absolute path to dirname inside the root of
// the repository, creating it if it doesn't exist.
func getRepoRootSubdir(dirname string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(root, dirname)
	err = os.MkdirAll(fullPath, 0755)

	return fullPath, err
}

var repoRootDir string

// repoRoot is a wrapper around git.FindRepoRoot(), but caches it so that
// multiple calls in the same invocation of the program won't run git multiple
// times.
func repoRoot() (string, error) {
	if repoRootDir != "" {
		return repoRootDir, nil
	}

	var err error
	repoRootDir, err = findRepoRoot(context.Background())
	return repoRootDir, err
}

// findRepoRoot returns the filesystem path of the root of the git repository
// that contains the process’s current directory.
func findRepoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "failed to find git repo root folder: %s", string(output))
	}

	return string(bytes.TrimSpace(output)), nil
}

func executableExistsWithVersion(ctx *task.Context, exe, exeVersion string) (bool, error) {
	exists, err := fileExists(exe)
	if err != nil {
		return false, err
	}
	if exists {
		var matches bool
		matches, err = versionMatches(ctx, exe, exeVersion)
		if err != nil {
			return false, err
		}
		if matches {
			return true, nil
		}
	}

	return false, nil
}

func fileExists(p string) (bool, error) {
	info, err := os.Stat(p)
	if err == nil && info.Mode().IsRegular() {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func versionMatches(ctx *task.Context, exe, wantVersion string) (bool, error) {
	out, err := sh.RunOutput(ctx, exe, "--version")
	if err != nil {
		return false, err
	}
	re := regexp.MustCompile(`\b\Q` + wantVersion + `\E\b`)

	return re.MatchString(out), nil
}

func httpGetWithRetries(url string, n int) (*http.Response, error) {
	var resp *http.Response
	var err error

	err = withRetries(
		n,
		fmt.Sprintf("Getting %s\n", url),
		func() error {
			resp, err = http.Get(url)
			return err
		},
	)

	return resp, err
}

// withRetries attempts to run cb up to n times, with exponential backoff. The
// backoff starts at 250ms and doubles each time, with a max of 32 seconds, so
// in practice you cannot retry something more than 9 times, which will take
// about a minute.
func withRetries(n int, what string, cb func() error) error {
	backoff := 250 * time.Millisecond
	var err error

	for i := 0; i < n; i++ {
		if i > 0 {
			fmt.Printf("sleeping %s before retrying %s after error: %s\n", backoff, what, err)
			time.Sleep(backoff)
			backoff *= 2
		}

		err = cb()
		if err == nil {
			return nil
		}

		// fail-safe
		if backoff > 32*time.Second {
			return errors.Wrapf(
				err,
				"%s failed to succeed before reaching max backoff duration",
				what,
			)
		}
	}

	return errors.Wrapf(err, "%s failed to succeed after %d times", what, n)
}

// doInDir changes directory to dirname and runs cb, then restores the original
// working directory before it returns.
func doInDir(dirname string, cb func() error) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	err = os.Chdir(dirname)
	if err != nil {
		return errors.Wrapf(err, "cannot change directory to %s", dirname)
	}

	defer func() {
		chdirErr := os.Chdir(wd)
		if chdirErr != nil {
			panic(
				fmt.Sprintf(
					"cannot change back to original working directory %s: %v",
					wd,
					chdirErr,
				),
			)
		}
	}()

	return cb()
}

func inCI() bool {
	return os.Getenv("EVG_BUILD_ID") != "" || os.Getenv("CI") != ""
}
