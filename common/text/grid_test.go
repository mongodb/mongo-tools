// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package text

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
)

func TestUpdateWidths(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	gw := GridWriter{}
	defaultWidths := []int{1, 2, 3, 4}

	// the first time, the grid's widths are nil
	assert.Nil(t, gw.colWidths)
	gw.updateWidths(defaultWidths)
	assert.Equal(t, defaultWidths, gw.colWidths)

	// the grid's widths should not be updated if all the new cell widths are less than or equal
	newWidths := []int{1, 2, 1, 2}
	assert.NotNil(t, gw.colWidths)
	gw.updateWidths(newWidths)
	assert.Equal(t, defaultWidths, gw.colWidths)
	assert.NotEqual(t, newWidths, gw.colWidths)

	// the grid's widths should be updated if any of the new cell widths are greater
	newWidths = []int{1, 2, 3, 5}
	assert.NotNil(t, gw.colWidths)
	gw.updateWidths(newWidths)
	assert.Equal(t, newWidths, gw.colWidths)
	assert.NotEqual(t, defaultWidths, gw.colWidths)
}

func writeData(gw *GridWriter) {
	gw.Reset()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			gw.WriteCell(fmt.Sprintf("(%v,%v)", i, j))
		}
		gw.EndRow()
	}
}

func TestWriteGrid(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("no min width", func(t *testing.T) {
		gw := new(GridWriter)
		writeData(gw)
		buf := bytes.Buffer{}
		gw.Flush(&buf)
		assert.Equal(
			t,
			"(0,0)(0,1)(0,2)\n(1,0)(1,1)(1,2)\n(2,0)(2,1)(2,2)\n",
			buf.String(),
		)

		writeData(gw)
		gw.MinWidth = 7
		buf = bytes.Buffer{}
		gw.Flush(&buf)
		assert.Contains(t, buf.String(),
			"  (0,0)  (0,1)  (0,2)\n  (1,0)  (1,1)")

		writeData(gw)
		gw.colWidths = []int{}
		gw.MinWidth = 0
		gw.ColumnPadding = 1
		buf = bytes.Buffer{}
		gw.Flush(&buf)
		assert.Contains(t, buf.String(),
			"(0,0) (0,1) (0,2)\n(1,0) (1,1)")

		writeData(gw)
		buf = bytes.Buffer{}
		gw.FlushRows(&buf)
		assert.Contains(t, buf.String(),
			"(0,0) (0,1) (0,2)(1,0) (1,1)")
	})

	t.Run("Test grid writer width calculation", func(t *testing.T) {
		gw := GridWriter{}
		gw.WriteCell("bbbb")
		gw.WriteCell("aa")
		gw.WriteCell("c")
		gw.EndRow()
		gw.WriteCell("bb")
		gw.WriteCell("a")
		gw.WriteCell("")
		gw.EndRow()
		assert.Equal(t, []int{4, 2, 1}, gw.calculateWidths())

		gw.WriteCell("bbbbbbb")
		gw.WriteCell("a")
		gw.WriteCell("cccc")
		gw.EndRow()
		assert.Equal(t, []int{7, 2, 4}, gw.calculateWidths())

		gw.WriteCell("bbbbbbb")
		gw.WriteCell("a")
		gw.WriteCell("cccc")
		gw.WriteCell("ddddddddd")
		gw.EndRow()
		assert.Equal(t, []int{7, 2, 4, 9}, gw.calculateWidths())
	})
}
