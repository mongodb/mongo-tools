// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build sharded

package mongotop

import (
	"slices"
	"testing"

	"github.com/mongodb/mongo-tools/internal/testcmd"
	"github.com/stretchr/testify/assert"
)

// TestMongotopRejectsMongos checks that mongotop refuses to run against a mongos, which does not
// support the top command, rather than failing later with whatever the mongos makes of it.
func TestMongotopRejectsMongos(t *testing.T) {
	const wantError = "cannot run mongotop against a mongos"

	for _, c := range []struct {
		name string
		args []string
	}{
		{"with no arguments", nil},
		{"with a sleep time", []string{"2"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, err := runMongotop(t, append(slices.Clone(c.args), "--rowcount", "1")...)
			testcmd.RequireExitFailure(t, err, "mongotop")
			assert.Contains(t, stderr, wantError, "the failure names the mongos")
		})
	}
}
