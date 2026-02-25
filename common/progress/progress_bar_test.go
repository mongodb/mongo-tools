// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build !race
// +build !race

// Disable race detector since these tests are inherently racy
package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
)

func TestBasicProgressBar(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	writeBuffer := &bytes.Buffer{}

	watching := NewCounter(10)
	pbar := &Bar{
		Name:      "\nTEST",
		Watching:  watching,
		WaitTime:  3 * time.Millisecond,
		Writer:    writeBuffer,
		BarLength: 10,
	}

	// Run progress bar while increment its counter

	pbar.Start()
	// TODO make this test non-racy and reliable
	time.Sleep(10 * time.Millisecond)
	// iterate though each value 1-10, sleeping to make sure it is written
	for localCounter := 0; localCounter < 10; localCounter++ {
		watching.Inc(1)
		time.Sleep(30 * time.Millisecond)
	}
	pbar.Stop()

	results := writeBuffer.String()
	assert.Contains(t, results, "TEST")
	assert.Contains(t, results, BarLeft)
	assert.Contains(t, results, BarRight)
	assert.Contains(t, results, BarFilling)
	assert.Contains(t, results, BarEmpty)
	assert.Contains(t, results, "0/10")
	assert.Contains(t, results, "1/10")
	assert.Contains(t, results, "2/10")
	assert.Contains(t, results, "3/10")
	assert.Contains(t, results, "4/10")
	assert.Contains(t, results, "5/10")
	assert.Contains(t, results, "6/10")
	assert.Contains(t, results, "7/10")
	assert.Contains(t, results, "8/10")
	assert.Contains(t, results, "9/10")
	assert.Contains(t, results, "10.0%")
}

func TestProgressBarWithNoMax(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	writeBuffer := &bytes.Buffer{}

	watching := NewCounter(0)
	watching.Inc(5)
	pbar := &Bar{
		Name:     "test",
		Watching: watching,
		Writer:   writeBuffer,
	}

	pbar.renderToWriter()
	assert.Contains(t, writeBuffer.String(), "5")
	assert.Contains(t, writeBuffer.String(), "test")
	assert.NotContains(t, writeBuffer.String(), "[")
	assert.NotContains(t, writeBuffer.String(), "]")
}

func TestBarConcurrency(t *testing.T) {
	// TOOLS-2715: Disable flaky test
	t.SkipNow()
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	writeBuffer := &bytes.Buffer{}

	watching := NewCounter(1000)
	watching.Inc(777)
	pbar := &Bar{
		Name:     "\nTEST",
		Watching: watching,
		WaitTime: 10 * time.Millisecond,
		Writer:   writeBuffer,
	}

	pbar.Start()
	time.Sleep(15 * time.Millisecond)
	watching.Inc(1)
	results := writeBuffer.String()
	assert.Contains(t, results, "777")
	assert.NotContains(t, results, "778")

	pbar.Stop()
	results = writeBuffer.String()
	assert.Contains(t, results, "777")
	assert.Contains(t, results, "778")

	assert.Panics(t, pbar.Start)
	assert.Panics(t, pbar.Stop)
}

func TestBarDrawing(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("20 wide @ 50%", func(t *testing.T) {
		b := drawBar(20, .5)
		assert.Equal(t, 10, strings.Count(b, BarFilling))
		assert.Equal(t, 10, strings.Count(b, BarEmpty))
		assert.Contains(t, b, BarLeft)
		assert.Contains(t, b, BarRight)
	})
	t.Run("100 wide @ 50%", func(t *testing.T) {
		b := drawBar(100, .5)
		assert.Equal(t, 50, strings.Count(b, BarFilling))
		assert.Equal(t, 50, strings.Count(b, BarEmpty))
	})
	t.Run("100 wide @ 99.9999%", func(t *testing.T) {
		b := drawBar(100, .999999)
		assert.Equal(t, 99, strings.Count(b, BarFilling))
		assert.Equal(t, 1, strings.Count(b, BarEmpty))
	})
	t.Run("9 wide @ 72%", func(t *testing.T) {
		b := drawBar(9, .72)
		assert.Equal(t, 6, strings.Count(b, BarFilling))
		assert.Equal(t, 3, strings.Count(b, BarEmpty))
	})
	t.Run("10 wide @ 0%", func(t *testing.T) {
		b := drawBar(10, 0)
		assert.Equal(t, 0, strings.Count(b, BarFilling))
		assert.Equal(t, 10, strings.Count(b, BarEmpty))
	})
	t.Run("10 wide @ 100%", func(t *testing.T) {
		b := drawBar(10, 1)
		assert.Equal(t, 10, strings.Count(b, BarFilling))
		assert.Equal(t, 0, strings.Count(b, BarEmpty))
	})
	t.Run("10 wide @ -60%", func(t *testing.T) {
		b := drawBar(10, -0.6)
		assert.Equal(t, 0, strings.Count(b, BarFilling))
		assert.Equal(t, 10, strings.Count(b, BarEmpty))
	})
	t.Run("10 wide @ 160%", func(t *testing.T) {
		b := drawBar(10, 1.6)
		assert.Equal(t, 10, strings.Count(b, BarFilling))
		assert.Equal(t, 0, strings.Count(b, BarEmpty))
	})
}

func TestBarUnits(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	writeBuffer := &bytes.Buffer{}

	watching := NewCounter(1024 * 1024)
	watching.Inc(777)
	pbar := &Bar{
		Name:     "\nTEST",
		Watching: watching,
		WaitTime: 10 * time.Millisecond,
		Writer:   writeBuffer,
		IsBytes:  true,
	}

	pbar.renderToWriter()
	assert.Contains(t, writeBuffer.String(), "B", "IsBytes writer returns bytes")
	assert.Contains(t, writeBuffer.String(), "MB", "IsBytes writer returns megabytes")
}
