// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package log

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verbosity struct {
	L int
	Q bool
}

func (v verbosity) IsQuiet() bool { return v.Q }
func (v verbosity) Level() int    { return v.L }

func TestBasicToolLoggerFunctionality(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	oldTime := time.Now()
	// sleep to avoid failures due to low timestamp resolution
	time.Sleep(time.Millisecond)

	tl := NewToolLogger(&verbosity{L: 3})
	require.NotNil(t, tl)
	assert.NotNil(t, tl.writer)
	assert.Equal(t, 3, tl.verbosity)

	assert.Panics(
		t,
		func() { tl.Logvf(-1, "nope") },
		"writing a negative verbosity panics",
	)

	buf := bytes.NewBuffer(make([]byte, 1024))
	tl.SetWriter(buf)

	// log at various verbosities
	tl.Logvf(0, "test this string")
	tl.Logvf(5, "this log level is too high and will not log")
	tl.Logvf(1, "====!%v!====", 12.5)

	l1, _ := buf.ReadString('\n')
	assert.Contains(t, l1, ":")
	assert.Contains(t, l1, ".")
	assert.Contains(t, l1, "test this string")

	l2, _ := buf.ReadString('\n')
	assert.Contains(t, l2, "====!12.5!====")

	require.Contains(t, l2, "\t")
	timestamp := l2[:strings.Index(l2, "\t")]
	assert.Greater(t, len(timestamp), 1)
	parsedTime, err := time.Parse(ToolTimeFormat, timestamp)
	require.NoError(t, err)
	assert.True(t, parsedTime.After(oldTime), "parsed time is on or after start time")
}

func TestGlobalToolLoggerFunctionality(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	globalToolLogger = nil // just to be sure

	globalToolLogger = NewToolLogger(&verbosity{L: 3})
	require.NotNil(t, globalToolLogger)

	assert.NotPanics(t, func() { SetVerbosity(&verbosity{Q: true}) })
	assert.NotPanics(t, func() { Logvf(0, "woooo") })
	assert.NotPanics(t, func() { SetDateFormat("ahaha") })
	assert.NotPanics(t, func() { SetWriter(os.Stdout) })
}

func TestToolLoggerWriter(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	buff := bytes.NewBuffer(make([]byte, 1024))
	tl := NewToolLogger(&verbosity{L: 3})
	tl.SetWriter(buff)

	t.Run("normal ToolLogWriter", func(t *testing.T) {
		tlw := tl.Writer(0)
		_, err := tlw.Write([]byte("One"))
		require.NoError(t, err)
		_, err = tlw.Write([]byte("Two"))
		require.NoError(t, err)
		_, err = tlw.Write([]byte("Three"))
		require.NoError(t, err)

		results := buff.String()
		assert.Contains(t, results, "One")
		assert.Contains(t, results, "Two")
		assert.Contains(t, results, "Three")
	})

	t.Run("log writer of too high verbosity", func(t *testing.T) {
		tlw2 := tl.Writer(1776)
		_, err := tlw2.Write([]byte("nothing to see here"))
		require.NoError(t, err)

		results := buff.String()
		assert.NotContains(t, results, "nothing")
	})
}
