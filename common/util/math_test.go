// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"reflect"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNumberConverter(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	floatConverter := newNumberConverter(reflect.TypeOf(float32(0)))

	t.Run("numeric values", func(t *testing.T) {
		out, err := floatConverter(21)
		require.NoError(t, err)
		assert.Equal(t, float32(21.0), out)

		out, err = floatConverter(uint64(21))
		require.NoError(t, err)
		assert.Equal(t, float32(21.0), out)

		out, err = floatConverter(float64(27.52))
		require.NoError(t, err)
		// There may be some floating point rounding errors so we cannot
		// compare the values exactly.
		assert.InDelta(t, float32(27.52), out, 0.000001)
	})

	t.Run("non-numeric values", func(t *testing.T) {
		_, err := floatConverter("I AM A STRING")
		require.Error(t, err)
		_, err = floatConverter(struct{ int }{12})
		require.Error(t, err)
		_, err = floatConverter(nil)
		require.Error(t, err)
	})
}

func TestUInt32Converter(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("numeric values", func(t *testing.T) {
		out, err := ToUInt32(int64(99))
		require.NoError(t, err)
		assert.Equal(t, uint32(99), out)

		out, err = ToUInt32(int32(99))
		require.NoError(t, err)
		assert.Equal(t, uint32(99), out)

		out, err = ToUInt32(float32(99))
		require.NoError(t, err)
		assert.Equal(t, uint32(99), out)

		out, err = ToUInt32(float64(99))
		require.NoError(t, err)
		assert.Equal(t, uint32(99), out)

		out, err = ToUInt32(uint64(99))
		require.NoError(t, err)
		assert.Equal(t, uint32(99), out)

		out, err = ToUInt32(uint32(99))
		require.NoError(t, err)
		assert.Equal(t, uint32(99), out)
	})

	t.Run("non-numeric ivalues", func(t *testing.T) {
		_, err := ToUInt32(nil)
		require.Error(t, err)
		_, err = ToUInt32("string")
		require.Error(t, err)
		_, err = ToUInt32([]byte{1, 2, 3, 4})
		require.Error(t, err)
	})
}
