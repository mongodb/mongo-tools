// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongotop

import (
	"fmt"
	"strconv"

	"github.com/mongodb/mongo-tools/common/options"
)

var Usage = `<options> <connection-string> <polling interval in seconds>

Monitor basic usage statistics for each collection.

Connection strings must begin with mongodb:// or mongodb+srv://.

See http://docs.mongodb.com/database-tools/mongotop/ for more information.`

type Options struct {
	*options.ToolOptions
	*Output
	SleepTime int
}

// Output defines the set of options to use in displaying data from the server.
type Output struct {
	Locks    bool `long:"locks" description:"report on use of per-database locks"`
	RowCount int  `long:"rowcount" value-name:"<count>" short:"n" description:"number of stats lines to print (0 for indefinite)"`
	Json     bool `long:"json" description:"format output as JSON"`
}

// Name returns a human-readable group name for output options.
func (_ *Output) Name() string {
	return "output"
}

func ParseOptions(rawArgs []string, versionStr, gitCommit string) (Options, error) {
	opts := options.New("mongotop", versionStr, gitCommit, Usage, true,
		options.EnabledOptions{Auth: true, Connection: true, Namespace: false, URI: true})
	opts.UseReadOnlyHostDescription()

	// add mongotop-specific options
	outputOpts := &Output{}
	opts.AddOptions(outputOpts)

	extraArgs, err := opts.ParseArgs(rawArgs)
	if err != nil {
		return Options{}, err
	}

	if len(extraArgs) > 1 {
		return Options{}, fmt.Errorf("error parsing positional arguments: " +
			"provide only one polling interval in seconds and only one MongoDB connection string. " +
			"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
		)
	}

	sleeptime := 1 // default to 1 second sleep time
	if len(extraArgs) > 0 {
		sleeptime, err = strconv.Atoi(extraArgs[0])
		if err != nil || sleeptime <= 0 {
			return Options{}, fmt.Errorf("invalid sleep time: %v", extraArgs[0])
		}
	}

	return Options{opts, outputOpts, sleeptime}, nil
}
