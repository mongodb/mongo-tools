// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build failpoints
// +build failpoints

package failpoint

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
)

func TestFailpointParsing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	args := "foo=bar,baz,biz=,=a"
	ParseFailpoints(args)

	assert.True(t, Enabled("foo"))
	assert.True(t, Enabled("baz"))
	assert.True(t, Enabled("biz"))
	assert.True(t, Enabled(""))
	assert.False(t, Enabled("bar"))

	val, ok := Get("foo")
	assert.Equal(t, "bar", val)
	assert.True(t, ok)

	val, ok = Get("baz")
	assert.Equal(t, "", val)
	assert.True(t, ok)

	val, ok = Get("biz")
	assert.Equal(t, "", val)
	assert.True(t, ok)

	val, ok = Get("")
	assert.Equal(t, "a", val)
	assert.True(t, ok)

	_, ok = Get("bar")
	assert.False(t, ok)
}
