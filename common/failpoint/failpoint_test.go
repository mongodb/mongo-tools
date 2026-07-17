// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build failpoints

package failpoint

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailpointParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	require.NoError(t, DefaultManager.Parse(""))
	defer DefaultManager.Reset()

	require.NoError(
		t,
		DefaultManager.Parse(string(PauseBeforeDumping)+","+string(SlowBSONDump)),
	)

	_, ok := DefaultManager.Get(PauseBeforeDumping)
	assert.True(t, ok)

	_, ok = DefaultManager.Get(SlowBSONDump)
	assert.True(t, ok)

	_, ok = DefaultManager.Get("NotARealFailpoint")
	assert.False(t, ok)

	err := DefaultManager.Parse("NotARealFailpoint")
	assert.ErrorContains(t, err, "unknown failpoint")
}
