// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongoexport tool.
package main

import (
	"os"

	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/signals"
	"github.com/mongodb/mongo-tools-common/util"
	"github.com/mongodb/mongo-tools/mongoexport"
)

var (
	VersionStr = "built-without-version-string"
	GitCommit  = "build-without-git-commit"
)

func main() {
	opts, err := mongoexport.ParseOptions(os.Args[1:], VersionStr, GitCommit)
	if err != nil {
		log.Logvf(log.Error, false, "error parsing command line options: %v", err)
		log.Logvf(log.Info, false, util.ShortUsage("mongoexport"))
		os.Exit(util.ExitFailure)
	}

	signals.Handle()

	// print help, if specified
	if opts.PrintHelp(false) {
		return
	}

	// print version, if specified
	if opts.PrintVersion() {
		return
	}

	exporter, err := mongoexport.New(opts)
	if err != nil {
		log.Logvf(log.Error, false, "%v", err)

		if se, ok := err.(util.SetupError); ok && se.Message != "" {
			log.Logv(log.Error, false, se.Message)
		}

		os.Exit(util.ExitFailure)
	}
	defer exporter.Close()

	writer, err := exporter.GetOutputWriter()
	if err != nil {
		log.Logvf(log.Error, false, "error opening output stream: %v", err)
		os.Exit(util.ExitFailure)
	}
	if writer == nil {
		writer = os.Stdout
	} else {
		defer writer.Close()
	}

	numDocs, err := exporter.Export(writer)
	if err != nil {
		log.Logvf(log.Error, false, "Failed: %v", err)
		os.Exit(util.ExitFailure)
	}

	if numDocs == 1 {
		log.Logvf(log.Info, false, "exported %v record", numDocs)
	} else {
		log.Logvf(log.Info, false, "exported %v records", numDocs)
	}

}
