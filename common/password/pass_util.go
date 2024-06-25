// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build !solaris
// +build !solaris

package password

import (
	"io"
	"os"

	"golang.org/x/term"
)

// This file contains all the calls needed to properly
// handle password input from stdin/terminal on all
// operating systems that aren't solaris

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func readPassInteractively() (string, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	//nolint:errcheck
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stderr}

	t := term.NewTerminal(screen, "")
	pass, err := t.ReadPassword("")
	if err != nil {
		return "", err
	}
	return pass, nil
}
