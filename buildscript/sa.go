package buildscript

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
)

func SAPreciousLint(ctx *task.Context) error {
	return runPrecious(ctx, "lint", "--all")
}

func runPrecious(ctx *task.Context, args ...string) error {
	cmd := append(
		[]string{"mise", "exec", "github:houseabsolute/precious", "--", "precious"},
		args...,
	)

	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return sh.RunCmd(ctx, c)
}

// SAModTidy runs go mod tidy and ensures it did not change go.mod or go.sum.
// Modeled on mongosync's Check.GeneratedFiles target:
// https://github.com/10gen/mongosync/blob/b5e64e9ad/magefiles/check.go#L37
func SAModTidy(ctx *task.Context) error {
	if err := sh.Run(ctx, "go", "mod", "tidy"); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--exit-code", "--", "go.mod", "go.sum")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := sh.RunCmd(ctx, cmd); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return errors.New(
				"go.mod and/or go.sum needs changes: run `go mod tidy` and commit the changes",
			)
		}
		return fmt.Errorf("error running `git diff --exit-code -- go.mod go.sum`: %w", err)
	}

	return nil
}

var modulesTxtRE = regexp.MustCompile(`(?m)^vendor/modules\.txt$`)

// SACheckVendoredCode checks that vendored code does not have any unexpected changes. An
// "unexpected" change is one that is not accompanied by a change to `vendor/modules.txt`.
//
// The goal of this check is to catch debugging changes made locally to vendored code that get
// accidentally committed as part of a PR.
func SACheckVendoredCode(ctx *task.Context) error {
	base := os.Getenv("EVG_BRANCH_NAME")
	if base == "" {
		base = "master"
	}

	refspec := fmt.Sprintf("%s...HEAD", base)

	out, err := sh.RunOutput(ctx, "git", "diff", "--name-only", refspec, "vendor")
	if err != nil {
		return fmt.Errorf("error running `git diff --name-only %s vendor`: %w", refspec, err)
	}

	if out == "" {
		return nil
	}

	if modulesTxtRE.MatchString(out) {
		return nil
	}

	return errors.New(
		"there is a change in vendor/ but not in vendor/modules.txt;" +
			" does this PR contain some debugging cruft?",
	)
}
