// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package progress

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type safeBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}

func (b *safeBuffer) String() string {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.String()
}

func (b *safeBuffer) Reset() {
	b.Lock()
	defer b.Unlock()
	b.Buffer.Reset()
}

func TestManagerAttachAndDetach(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	setup := func() (*BarWriter, *safeBuffer) {
		writeBuffer := new(safeBuffer)
		manager := NewBarWriter(writeBuffer, time.Second, 10, false)
		require.NotNil(t, manager)

		progressor := NewCounter(10)
		progressor.Inc(5)
		manager.Attach("TEST1", progressor)
		manager.Attach("TEST2", progressor)
		manager.Attach("TEST3", progressor)

		return manager, writeBuffer
	}

	t.Run("adding 3 bars", func(t *testing.T) {
		manager, writeBuffer := setup()
		assert.Len(t, manager.bars, 3)

		manager.renderAllBars()
		writtenString := writeBuffer.String()
		assert.Contains(t, writtenString, "TEST1")
		assert.Contains(t, writtenString, "TEST2")
		assert.Contains(t, writtenString, "TEST3")
	})

	t.Run("detaching the second bar", func(t *testing.T) {
		manager, writeBuffer := setup()
		assert.Len(t, manager.bars, 3)

		manager.Detach("TEST2")
		assert.Len(t, manager.bars, 2)

		manager.renderAllBars()
		writtenString := writeBuffer.String()
		assert.Contains(t, writtenString, "TEST1")
		assert.NotContains(t, writtenString, "TEST2")
		assert.Contains(t, writtenString, "TEST3")
		assert.Less(
			t,
			strings.Index(writtenString, "TEST1"),
			strings.Index(writtenString, "TEST3"),
		)

		writeBuffer.Reset()

		t.Run("adding a new bar after should print 1,3,4", func(t *testing.T) {
			manager.Attach("TEST4", NewCounter(10))
			assert.Len(t, manager.bars, 3)

			manager.renderAllBars()
			writtenString := writeBuffer.String()
			assert.Contains(t, writtenString, "TEST1")
			assert.NotContains(t, writtenString, "TEST2")
			assert.Contains(t, writtenString, "TEST3")
			assert.Contains(t, writtenString, "TEST4")

			assert.Less(
				t,
				strings.Index(writtenString, "TEST1"),
				strings.Index(writtenString, "TEST3"),
			)
			assert.Less(
				t,
				strings.Index(writtenString, "TEST3"),
				strings.Index(writtenString, "TEST4"),
			)
		})
	})
}

func TestManagerStartAndStop(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	writeBuffer := new(safeBuffer)

	manager := NewBarWriter(writeBuffer, 10*time.Millisecond, 10, false)
	require.NotNil(t, manager)

	watching := NewCounter(10)
	watching.Inc(5)
	manager.Attach("TEST", watching)

	assert.Equal(t, 10*time.Millisecond, manager.waitTime)
	assert.Len(t, manager.bars, 1)

	t.Run("running the manager for 100 ms and stopping", func(t *testing.T) {
		manager.Start()
		// enough time for the manager to write at least 4 times
		time.Sleep(time.Millisecond * 100)
		manager.Stop()

		output := writeBuffer.String()
		assert.GreaterOrEqual(t, strings.Count(output, "TEST"), 4)
	})

	t.Run("start+stop the manager again", func(t *testing.T) {
		assert.NotPanics(t, manager.Start)
		assert.NotPanics(t, manager.Stop)
	})
}

func TestNumberOfWrites(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	setup := func() (*BarWriter, *CountWriter) {
		cw := new(CountWriter)
		manager := NewBarWriter(cw, time.Millisecond*10, 10, false)
		require.NotNil(t, manager)

		manager.Attach("1", NewCounter(10))
		return manager, cw
	}

	t.Run("with one attached bar", func(t *testing.T) {
		manager, cw := setup()
		assert.Len(t, manager.bars, 1)

		manager.renderAllBars()
		assert.Equal(t, 1, cw.Count(), "one write per render")
	})

	t.Run("with two bars attached", func(t *testing.T) {
		manager, cw := setup()
		manager.Attach("2", NewCounter(10))
		assert.Len(t, manager.bars, 2)

		manager.renderAllBars()
		assert.Equal(t, 3, cw.Count(), "one write per render plus an empty write")
	})

	t.Run("with 57 bars attached", func(t *testing.T) {
		manager, cw := setup()
		for i := 2; i <= 57; i++ {
			manager.Attach(strconv.Itoa(i), NewCounter(10))
		}
		assert.Len(t, manager.bars, 57)

		manager.renderAllBars()
		assert.Equal(t, 58, cw.Count(), "one write per render plus an empty write")
	})
}

// helper type for counting calls to a writer.
type CountWriter int

func (cw CountWriter) Count() int {
	return int(cw)
}

func (cw *CountWriter) Write(b []byte) (int, error) {
	*cw++
	return len(b), nil
}
