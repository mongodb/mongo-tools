package buildscript

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/pkg/errors"
)

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
