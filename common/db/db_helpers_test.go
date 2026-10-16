// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"testing"

	"github.com/mongodb/mongo-tools/common/options"
	"github.com/stretchr/testify/require"
)

// configureClient starts background topology monitoring, so a client that is
// never disconnected leaves goroutines resolving hostnames for the rest of the
// package's test run. It is shared by unit and integration tests, so it lives
// in an untagged file.
func configureClientAndDisconnectAtEnd(t *testing.T, opts options.ToolOptions) error {
	client, err := configureClient(opts)
	if client != nil {
		t.Cleanup(func() {
			require.NoError(t, client.Disconnect(context.Background()), "disconnect client")
		})
	}

	return err
}
