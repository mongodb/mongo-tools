package buildscript

import (
	"bytes"
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

// SAModTidy runs go mod tidy and ensure no changes were made.
// Copied from mongohouse: https://github.com/10gen/mongohouse/blob/333308814f96a0909c8125f71af7748b263e3263/buildscript/sa.go#L72
func SAModTidy(ctx *task.Context) error {
	// Save original contents in case they get modified. When
	// https://github.com/golang/go/issues/27005 is done, we
	// shouldn't need this anymore.
	origGoMod, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}
	origGoSum, err := os.ReadFile("go.sum")
	if err != nil {
		return fmt.Errorf("error reading go.sum: %w", err)
	}

	err = sh.Run(ctx, "go", "mod", "tidy")
	if err != nil {
		return err
	}

	newGoMod, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}
	newGoSum, err := os.ReadFile("go.sum")
	if err != nil {
		return fmt.Errorf("error reading go.sum: %w", err)
	}

	if !bytes.Equal(origGoMod, newGoMod) || !bytes.Equal(origGoSum, newGoSum) {
		// Restore originals, ignoring errors since they need tidying anyway.
		_ = os.WriteFile("go.mod", origGoMod, 0600)
		_ = os.WriteFile("go.sum", origGoSum, 0600)
		return errors.New(
			"go.mod and/or go.sum needs changes: run `go mod tidy` and commit the changes",
		)
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
