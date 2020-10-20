package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/craiggwilson/goke/task"
)

// Branch returns the current branch of the git repository in the current working directory.
func Branch(ctx *task.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	ctx.Logf("exec: '%s %s'\n", cmd.Path, strings.Join(cmd.Args[1:], " "))
	output, err := cmd.CombinedOutput()
	return string(bytes.TrimSpace(output)), err
}

// SHA1 returns the current sha1 for the HEAD commit of the git repository in the current working directory.
func SHA1(ctx *task.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	ctx.Logf("exec: '%s %s'\n", cmd.Path, strings.Join(cmd.Args[1:], " "))
	output, err := cmd.CombinedOutput()
	return string(bytes.TrimSpace(output)), err
}

// TagAndCommitsSince returns the latest tag on the current branch and the number of commits since the tag
// of the git repository in the current working directory.
func TagAndCommitsSince(ctx *task.Context, defaultTag string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--abbrev=0")
	ctx.Logf("exec: '%s %s'\n", cmd.Path, strings.Join(cmd.Args[1:], " "))
	tagName, err := cmd.CombinedOutput()
	if err != nil {
		cmd := exec.CommandContext(ctx, "git", "rev-list", "HEAD", "--count")
		ctx.Logf("exec: '%s %s'\n", cmd.Path, strings.Join(cmd.Args[1:], " "))
		commitsSince, err := cmd.CombinedOutput()
		if err != nil {
			return "", "", err
		}

		commitsSince = bytes.TrimSpace(commitsSince)
		return defaultTag, string(commitsSince), nil
	}

	tagName = bytes.TrimSpace(tagName)

	cmd = exec.CommandContext(ctx, "git", "rev-list", fmt.Sprintf("%s..HEAD", string(tagName)), "--count")
	ctx.Logf("exec: '%s %s'\n", cmd.Path, strings.Join(cmd.Args[1:], " "))

	commitsSince, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", err
	}

	commitsSince = bytes.TrimSpace(commitsSince)

	return string(tagName), string(commitsSince), nil
}
