// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package failpoint

import (
	"fmt"
	"strings"
)

// Name identifies a failpoint that can be turned on via --failpoints.
type Name string

func (n Name) String() string { return string(n) }

// validNames holds every failpoint declared via newName below, so that Parse
// can reject unrecognized names instead of silently accepting typos.
var validNames = map[Name]bool{}

func newName(s string) Name {
	n := Name(s)
	validNames[n] = true
	return n
}

// Supported failpoints.
var (
	PauseBeforeDumping = newName("PauseBeforeDumping")
	SlowBSONDump       = newName("SlowBSONDump")
	PauseUntilResumed  = newName("PauseUntilResumed")
)

// parseNames splits arg, a comma-separated list of failpoint names as
// passed to --failpoints, and returns an error if any of them isn't a known
// failpoint. An empty arg (the default, when --failpoints isn't passed) is
// valid and returns no names.
func parseNames(arg string) ([]Name, error) {
	if arg == "" {
		return nil, nil
	}

	parts := strings.Split(arg, ",")
	names := make([]Name, len(parts))
	for i, part := range parts {
		name := Name(part)
		if !validNames[name] {
			return nil, fmt.Errorf("unknown failpoint: %q", part)
		}
		names[i] = name
	}
	return names, nil
}
