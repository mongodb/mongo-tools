// Copyright (C) MongoDB, Inc. 2025-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package mongodump

import (
	"os"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
    "github.com/pkg/errors"
)

// Wait until a file exists and can be opened for reading
// This is used only for testing mongodump/mongorestore with resmoke
// test infrastructure.  The tests will create the barrier file when they have
// finshed writes to the source cluster.
func waitForSourceWritesDoneBarrier(barrierName string) {
	log.Logvf(log.DebugHigh, "waitForSourceWritesDoneBarrier: initial check for existence of file %s", barrierName)
	start := time.Now()
	logInterval := time.Minute
	prevLogTime := start
	for {
		f, err := os.Open(barrierName)
		if err == nil {
			// We opened the file for reading, so it does exist.
			f.Close()
	        log.Logvf(log.DebugHigh, "waitForSourceWritesDoneBarrier: barrier file %s exists - proceed past the barrier", barrierName)
			return
		}
		if os.IsNotExist(err) {
			if time.Since(prevLogTime) >= logInterval {
				prevLogTime = time.Now()
				log.Logvf(log.DebugHigh, "waitForSourceWritesDoneBarrier: still waiting for existence of file %s after %.1f sec",
					barrierName,
					prevLogTime.Sub(start).Seconds())
			}
			// Poll for existence of the barrier file every 500msec
			time.Sleep(500 * time.Millisecond)
		} else {
			panic(errors.Wrapf(err, "failed to open barrier file %s", barrierName))
		}
	}
}
