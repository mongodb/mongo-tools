package buildscript

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
)

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
	output, err := sh.RunOutput(ctx, "evergreen", "validate", "--file", "common.yml")
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
