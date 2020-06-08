// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongostat

import (
	"fmt"
	"strconv"

	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer"
)

var Usage = `<options> <connection-string> <polling interval in seconds>

Monitor basic MongoDB server statistics.

Connection strings must begin with mongodb:// or mongodb+srv://.

See http://docs.mongodb.com/database-tools/mongostat/ for more information.`

// StatOptions defines the set of options to use for configuring mongostat.
type StatOptions struct {
	Columns       string `short:"o" value-name:"<field>[,<field>]*" description:"fields to show. For custom fields, use dot-syntax to index into serverStatus output, and optional methods .diff() and .rate() e.g. metrics.record.moves.diff()"`
	AppendColumns string `short:"O" value-name:"<field>[,<field>]*" description:"like -o, but preloaded with default fields. Specified fields inserted after default output"`
	HumanReadable string `long:"humanReadable" default:"true" description:"print sizes and time in human readable format (e.g. 1K 234M 2G). To use the more precise machine readable format, use --humanReadable=false"`
	NoHeaders     bool   `long:"noheaders" description:"don't output column names"`
	RowCount      int64  `long:"rowcount" value-name:"<count>" short:"n" description:"number of stats lines to print (0 for indefinite)"`
	Discover      bool   `long:"discover" description:"discover nodes and display stats for all"`
	Http          bool   `long:"http" description:"use HTTP instead of raw db connection"`
	All           bool   `long:"all" description:"all optional fields"`
	Json          bool   `long:"json" description:"output as JSON rather than a formatted table"`
	Deprecated    bool   `long:"useDeprecatedJsonKeys" description:"use old key names; only valid with the json output option."`
	Interactive   bool   `short:"i" long:"interactive" description:"display stats in a non-scrolling interface"`
}

// Name returns a human-readable group name for mongostat options.
func (*StatOptions) Name() string {
	return "stat"
}

type Options struct {
	*options.ToolOptions
	*StatOptions
	SleepInterval int
}

func ParseOptions(rawArgs []string, versionStr, gitCommit string) (Options, error) {
	opts := options.New(
		"mongostat", versionStr, gitCommit, Usage, true,
		options.EnabledOptions{Connection: true, Auth: true, Namespace: false, URI: true})
	opts.UseReadOnlyHostDescription()

	// add mongostat-specific options
	statOpts := &StatOptions{}
	opts.AddOptions(statOpts)

	interactiveOption := opts.FindOptionByLongName("interactive")
	if _, available := stat_consumer.FormatterConstructors["interactive"]; !available {
		// make --interactive inaccessible
		interactiveOption.LongName = ""
		interactiveOption.ShortName = 0
	}

	args, err := opts.ParseArgs(rawArgs)
	if err != nil {
		return Options{}, err
	}

	if len(args) > 1 {
		return Options{}, fmt.Errorf("error parsing positional arguments: " +
			"provide only one polling interval in seconds and only one MongoDB connection string. " +
			"Connection strings must begin with mongodb:// or mongodb+srv:// schemes",
		)
	}

	sleepInterval := 1
	if len(args) == 1 {
		sleepInterval, err = strconv.Atoi(args[0])
		if err != nil {
			return Options{}, fmt.Errorf("invalid sleep interval: %v", args[0])
		}
		if sleepInterval < 1 {
			return Options{}, fmt.Errorf("sleep interval must be at least 1 second")
		}
	}

	return Options{opts, statOpts, sleepInterval}, nil
}
