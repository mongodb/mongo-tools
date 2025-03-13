// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package mongodump

import (
	"os"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
)

// Wait until a file exists and can be opened for reading
// This is used only for testing mongodump/mongorestore with resmoke
// test infrastructure.  The tests will create the barrier file when they have
// finshed writes to the source cluster.
func waitForSourceWritesDoneBarrier(barrierName string) {
	log.Logvf(log.Always, "waitForSourceWritesDoneBarrier: %s", barrierName)
	start := time.Now()
	logInterval := time.Minute
	prevLogTime := start
	for {
		f, err := os.Open(barrierName)
		if err == nil {
			// We opened the file for reading, so it does exist.
			f.Close()
			return
		}
		if os.IsNotExist(err) {
			if time.Since(prevLogTime) >= logInterval {
				prevLogTime = time.Now()
				log.Logvf(log.Always, "waitForSourceWritesDoneBarrier: waited %.1f secs for %s",
					prevLogTime.Sub(start).Seconds(),
					barrierName)
			}
			// Poll for existence of the barrier file every 500msec
			time.Sleep(500 * time.Millisecond)
		} else {
			panic(err)
		}
	}
}
