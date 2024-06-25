// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongodump tool.
package main

import (
	"os"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/signals"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongodump"
)

const (
	progressBarLength   = 24
	progressBarWaitTime = time.Second * 3
)

var (
	VersionStr = "built-without-version-string"
	GitCommit  = "build-without-git-commit"
)

func main() {
	// initialize command-line opts
	opts, err := mongodump.ParseOptions(os.Args[1:], VersionStr, GitCommit)
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %s", err.Error())
		log.Logvf(log.Always, util.ShortUsage("mongodump"))
		os.Exit(util.ExitFailure)
	}

	// print help, if specified
	if opts.PrintHelp(false) {
		return
	}

	// print version, if specified
	if opts.PrintVersion() {
		return
	}

	// init logger
	log.SetVerbosity(opts.Verbosity)

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	// kick off the progress bar manager
	progressManager := progress.NewBarWriter(
		log.Writer(0),
		progressBarWaitTime,
		progressBarLength,
		false,
	)
	progressManager.Start()
	defer progressManager.Stop()

	dump := mongodump.MongoDump{
		ToolOptions:     opts.ToolOptions,
		OutputOptions:   opts.OutputOptions,
		InputOptions:    opts.InputOptions,
		ProgressManager: progressManager,
	}

	finishedChan := signals.HandleWithInterrupt(dump.HandleInterrupt)
	defer close(finishedChan)

	if err = dump.Init(); err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitFailure)
	}

	if err = dump.Dump(); err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitFailure)
	}
}
