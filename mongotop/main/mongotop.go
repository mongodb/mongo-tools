// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongotop tool.
package main

import (
	"os"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/signals"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongotop"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	VersionStr = "built-without-version-string"
	GitCommit  = "build-without-git-commit"
)

func main() {
	// initialize command-line opts
	opts, err := mongotop.ParseOptions(os.Args[1:], VersionStr, GitCommit)
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %s", err.Error())
		log.Logvf(log.Always, util.ShortUsage("mongotop"))
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

	log.SetVerbosity(opts.Verbosity)
	signals.Handle()

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	if opts.RowCount < 0 {
		log.Logvf(log.Always, "invalid value for --rowcount: %v", opts.RowCount)
		os.Exit(util.ExitFailure)
	}

	if opts.Auth.Username != "" && opts.Auth.Source == "" && !opts.Auth.RequiresExternalDB() {
		if opts.URI != nil && opts.URI.ConnectionString != "" {
			log.Logvf(
				log.Always,
				"authSource is required when authenticating against a non $external database",
			)
			os.Exit(util.ExitFailure)
		}
		log.Logvf(
			log.Always,
			"--authenticationDatabase is required when authenticating against a non $external database",
		)
		os.Exit(util.ExitFailure)
	}

	if opts.ReplicaSetName == "" {
		opts.ReadPreference = readpref.PrimaryPreferred()
	}

	// create a session provider to connect to the db
	sessionProvider, err := db.NewSessionProvider(*opts.ToolOptions)
	if err != nil {
		log.Logvf(log.Always, "error connecting to host: %v", err)
		os.Exit(util.ExitFailure)
	}

	// fail fast if connecting to a mongos
	isMongos, err := sessionProvider.IsMongos()
	if err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitFailure)
	}
	if isMongos {
		log.Logvf(log.Always, "cannot run mongotop against a mongos")
		os.Exit(util.ExitFailure)
	}

	// instantiate a mongotop instance
	top := &mongotop.MongoTop{
		Options:         opts.ToolOptions,
		OutputOptions:   opts.Output,
		SessionProvider: sessionProvider,
		Sleeptime:       time.Duration(opts.SleepTime) * time.Second,
	}

	// kick it off
	if err := top.Run(); err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitFailure)
	}
}
