// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"github.com/jessevdk/go-flags"
	"github.com/mongodb/mongo-tools/legacy/options"
	"github.com/mongodb/mongo-tools/mongoreplay"

	"fmt"
	"os"
	"runtime"
)

const (
	ExitOk       = 0
	ExitError    = 1
	ExitNonFatal = 3
	// Go reserves exit code 2 for its own use
)

var (
	VersionStr = "built-without-version-string"
	GitCommit  = "build-without-git-commit"
)

func main() {
	versionOpts := mongoreplay.VersionOptions{}
	versionFlagParser := flags.NewParser(&versionOpts, flags.Default)
	versionFlagParser.Options = flags.IgnoreUnknown
	_, err := versionFlagParser.Parse()
	if err != nil {
		os.Exit(ExitError)
	}

	if versionOpts.PrintVersion(VersionStr, GitCommit) {
		os.Exit(ExitOk)
	}

	if runtime.NumCPU() == 1 {
		fmt.Fprint(os.Stderr, "mongoreplay must be run with multiple threads")
		os.Exit(ExitError)
	}

	opts := mongoreplay.Options{VersionStr: VersionStr, GitCommit: GitCommit}

	var parser = flags.NewParser(&opts, flags.Default)

	playCmd := &mongoreplay.PlayCommand{GlobalOpts: &opts}
	playCmdParser, err := parser.AddCommand("play", "Play captured traffic against a mongodb instance", "", playCmd)
	if err != nil {
		panic(err)
	}
	if options.BuiltWithSSL {
		playCmd.SSLOpts = &options.SSL{}
		_, err := playCmdParser.AddGroup("ssl", "", playCmd.SSLOpts)
		if err != nil {
			panic(err)
		}
	}

	_, err = parser.AddCommand("record", "Convert network traffic into mongodb queries", "",
		&mongoreplay.RecordCommand{GlobalOpts: &opts})
	if err != nil {
		panic(err)
	}

	_, err = parser.AddCommand("monitor", "Inspect live or pre-recorded mongodb traffic", "",
		&mongoreplay.MonitorCommand{GlobalOpts: &opts})
	if err != nil {
		panic(err)
	}

	_, err = parser.AddCommand("filter", "Filter playback file", "",
		&mongoreplay.FilterCommand{GlobalOpts: &opts})
	if err != nil {
		panic(err)
	}

	_, err = parser.Parse()

	if err != nil {
		switch err.(type) {
		case mongoreplay.ErrPacketsDropped:
			os.Exit(ExitNonFatal)
		default:
			os.Exit(ExitError)
		}
	}
}
