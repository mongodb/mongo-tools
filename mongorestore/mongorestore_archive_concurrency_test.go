// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"math"
	"runtime"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
)

func TestNumParallelCollectionsForArchive(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	maxParallelCollections := runtime.GOMAXPROCS(0) * maxParallelCollectionsPerProc

	tests := []struct {
		name                          string
		archiveConcurrentCollections  int
		currentNumParallelCollections int
		expected                      int
	}{
		{
			name:                          "archive value lower than current setting is ignored",
			archiveConcurrentCollections:  1,
			currentNumParallelCollections: 4,
			expected:                      4,
		},
		{
			name:                          "archive value higher than current setting raises it",
			archiveConcurrentCollections:  8,
			currentNumParallelCollections: 4,
			expected:                      8,
		},
		{
			name:                          "excessive archive value is capped",
			archiveConcurrentCollections:  math.MaxInt32,
			currentNumParallelCollections: 4,
			expected:                      maxParallelCollections,
		},
		{
			name:                          "capped archive value still under current setting is ignored",
			archiveConcurrentCollections:  math.MaxInt32,
			currentNumParallelCollections: maxParallelCollections + 1,
			expected:                      maxParallelCollections + 1,
		},
		{
			name:                          "negative archive value is ignored",
			archiveConcurrentCollections:  -1,
			currentNumParallelCollections: 4,
			expected:                      4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := numParallelCollectionsForArchive(
				tt.archiveConcurrentCollections,
				tt.currentNumParallelCollections,
			)
			assert.Equal(
				t,
				tt.expected,
				actual,
				"numParallelCollectionsForArchive(%d, %d) should return %d",
				tt.archiveConcurrentCollections,
				tt.currentNumParallelCollections,
				tt.expected,
			)
		})
	}
}
