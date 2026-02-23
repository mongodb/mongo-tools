// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
)

func TestFormatDate(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	validDates := []string{
		"2014-01-02T15:04:05.000Z",
		"2014-03-02T15:05:05Z",
		"2014-04-02T15:04Z",
		"2014-04-02T15:04-0800",
		"2014-04-02T15:04:05.000-0600",
		"2014-04-02T15:04:05-0500",
	}

	for _, date := range validDates {
		_, err := FormatDate(date)
		require.NoError(t, err, date)
	}

	// and one invalid test
	_, err := FormatDate("invalid string format")
	require.Error(t, err)
}
