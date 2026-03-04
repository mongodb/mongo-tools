// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongostat

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer/line"
	"github.com/mongodb/mongo-tools/mongostat/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func readBSONFile(file string, t *testing.T) (stat *status.ServerStatus) {
	stat = &status.ServerStatus{}
	ssBSON, err := os.ReadFile(file)
	require.NoError(t, err, "load ServerStatusBSON from file")

	err = bson.Unmarshal(ssBSON, stat)
	require.NoError(t, err, "unmarshal ServerStatusBSON")
	return
}

func TestStatLine(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	defaultHeaders := make([]string, len(line.CondHeaders))
	for i, h := range line.CondHeaders {
		defaultHeaders[i] = h.Key
	}
	defaultConfig := &status.ReaderConfig{
		HumanReadable: true,
	}

	serverStatusOld := readBSONFile("test_data/server_status_old.bson", t)
	serverStatusNew := readBSONFile("test_data/server_status_new.bson", t)
	serverStatusNew.ShardCursorType = nil
	serverStatusOld.ShardCursorType = nil

	t.Run("calculate opcounter diffs", func(t *testing.T) {
		statsLine := line.NewStatLine(
			serverStatusOld,
			serverStatusNew,
			defaultHeaders,
			defaultConfig,
		)
		assert.Equal(t, "10", statsLine.Fields["insert"])
		assert.Equal(t, "5", statsLine.Fields["query"])
		assert.Equal(t, "7", statsLine.Fields["update"])
		assert.Equal(t, "2", statsLine.Fields["delete"])
		assert.Equal(t, "3", statsLine.Fields["getmore"])
		command := strings.Split(statsLine.Fields["command"], "|")[0]
		assert.Equal(t, "669", command)

		locked := strings.Split(statsLine.Fields["locked_db"], ":")
		assert.Equal(t, "test", locked[0])
		assert.Equal(t, "50.0%", locked[1])
		qrw := strings.Split(statsLine.Fields["qrw"], "|")
		assert.Equal(t, "3", qrw[0])
		assert.Equal(t, "2", qrw[1])
		arw := strings.Split(statsLine.Fields["arw"], "|")
		assert.Equal(t, "4", arw[0])
		assert.Equal(t, "6", arw[1])
		assert.Equal(t, "2.00k", statsLine.Fields["net_in"])
		assert.Equal(t, "3.00k", statsLine.Fields["net_out"])
		assert.Equal(t, "5", statsLine.Fields["conn"])
	})

	serverStatusNew.SampleTime, _ = time.Parse("2006 Jan 02 15:04:05", "2015 Nov 30 4:25:33")
	t.Run("calculate average diffs", func(t *testing.T) {
		statsLine := line.NewStatLine(
			serverStatusOld,
			serverStatusNew,
			defaultHeaders,
			defaultConfig,
		)
		// Opcounters are averaged over sample period
		assert.Equal(t, "3", statsLine.Fields["insert"])
		assert.Equal(t, "1", statsLine.Fields["query"])
		assert.Equal(t, "2", statsLine.Fields["update"])
		deleted := strings.TrimPrefix(statsLine.Fields["delete"], "*")
		assert.Equal(t, "0", deleted)
		assert.Equal(t, "1", statsLine.Fields["getmore"])
		command := strings.Split(statsLine.Fields["command"], "|")[0]
		assert.Equal(t, "223", command)

		locked := strings.Split(statsLine.Fields["locked_db"], ":")
		assert.Equal(t, "test", locked[0])
		assert.Equal(t, "50.0%", locked[1])
		qrw := strings.Split(statsLine.Fields["qrw"], "|")
		assert.Equal(t, "3", qrw[0])
		assert.Equal(t, "2", qrw[1])
		arw := strings.Split(statsLine.Fields["arw"], "|")
		assert.Equal(t, "4", arw[0])
		assert.Equal(t, "6", arw[1])
		assert.Equal(t, "666b", statsLine.Fields["net_in"])
		assert.Equal(t, "1.00k", statsLine.Fields["net_out"])
		assert.Equal(t, "5", statsLine.Fields["conn"])
	})
}

func TestIsMongos(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tests := []struct {
		processName string
		expect      bool
	}{
		// true cases
		{"/mongos-prod.exe", true},
		{"/mongos.exe", true},
		{"mongos", true},
		{"mongodb/bin/mongos", true},
		{`C:\data\mci\48de1dc1ec3c2be5dcd6a53739578de4\src\mongos.exe`, true},

		// false cases
		{"mongosx/mongod", false},
		{"mongostat", false},
		{"mongos_stuff/mongod", false},
		{"mongos.stuff/mongod", false},
		{"mongodb/bin/mongod", false},
	}

	for _, test := range tests {
		got := status.IsMongos(&status.ServerStatus{Process: test.processName})
		assert.Equal(t, test.expect, got, "%s: expect mongos = %v", test.processName, test.expect)
	}
}
