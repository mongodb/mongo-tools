// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build integration || sharded

package options

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/stretchr/testify/require"
)

func TestLogUnsupportedOptions(t *testing.T) {
	var buffer bytes.Buffer

	log.SetWriter(&buffer)
	defer log.SetWriter(os.Stderr)

	t.Run("no unsupported options", func(t *testing.T) {
		args := []string{"mongodb://mongodb.test.com:27017"}

		enabled := EnabledOptions{true, true, true, true}
		opts := New("", "", "", "", true, enabled)

		_, err := opts.ParseArgs(args)
		require.NoError(t, err)

		opts.LogUnsupportedOptions()

		result := buffer.String()
		require.Empty(t, result)
	})

	t.Run("unsupported option", func(t *testing.T) {
		args := []string{"mongodb://mongodb.test.com:27017/?foo=bar"}

		enabled := EnabledOptions{true, true, true, true}
		opts := New("", "", "", "", true, enabled)

		_, err := opts.ParseArgs(args)
		require.NoError(t, err)

		opts.LogUnsupportedOptions()

		result := buffer.String()
		expectedResult := fmt.Sprintf(unknownOptionsWarningFormat, "foo")

		require.Contains(t, result, expectedResult)
	})
}
