// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package password handles cleanly reading in a user's password from
// the command line. This varies heavily between operating systems.
package password

import (
	"fmt"
	"io"
	"os"

	"github.com/mongodb/mongo-tools/common/log"
)

// key constants
const (
	backspaceKey      = 8
	deleteKey         = 127
	etxKey            = 3
	eotKey            = 4
	newLineKey        = 10
	carriageReturnKey = 13
)

// Prompt displays a prompt asking for the password and returns the
// password the user enters as a string.
func Prompt(what string) (string, error) {
	var pass string
	var err error
	if IsTerminal() {
		log.Logv(log.DebugLow, "standard input is a terminal; reading password from terminal")
		fmt.Fprintf(os.Stderr, "Enter password for %s:", what)
		pass, err = readPassInteractively()
	} else {
		log.Logv(log.Always, "reading password from standard input")
		fmt.Fprintf(os.Stderr, "Enter password for %s:", what)
		pass, err = readPassNonInteractively(os.Stdin)
	}
	if err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stderr)
	return pass, nil
}

// readPassNonInteractively pipes in a password from stdin if
// we aren't using a terminal for standard input
func readPassNonInteractively(reader io.Reader) (string, error) {
	pass := []byte{}
	for {
		var chBuf [1]byte
		n, err := reader.Read(chBuf[:])

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if n == 0 {
			break
		}
		ch := chBuf[0]
		if ch == backspaceKey || ch == deleteKey {
			if len(pass) > 0 {
				pass = pass[:len(pass)-1]
			}
		} else if ch == carriageReturnKey || ch == newLineKey || ch == etxKey || ch == eotKey {
			break
		} else if ch != 0 {
			pass = append(pass, ch)
		}
	}
	return string(pass), nil
}
