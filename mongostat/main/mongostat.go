// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongostat tool.
package main

import (
	"os"
	"strings"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/password"
	"github.com/mongodb/mongo-tools/common/signals"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongostat"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer/line"
	"github.com/mongodb/mongo-tools/mongostat/status"
)

// optionKeyNames interprets the CLI options Columns and AppendColumns into
// the internal keyName mapping.
func optionKeyNames(option string) map[string]string {
	kn := make(map[string]string)
	columns := strings.Split(option, ",")
	for _, column := range columns {
		naming := strings.Split(column, "=")
		if len(naming) == 1 {
			kn[naming[0]] = naming[0]
		} else {
			kn[naming[0]] = naming[1]
		}
	}
	return kn
}

// optionCustomHeaders interprets the CLI options Columns and AppendColumns
// into a list of custom headers.
func optionCustomHeaders(option string) (headers []string) {
	columns := strings.Split(option, ",")
	for _, column := range columns {
		naming := strings.Split(column, "=")
		headers = append(headers, naming[0])
	}
	return
}

var (
	VersionStr = "built-without-version-string"
	GitCommit  = "build-without-git-commit"
)

func main() {
	// initialize command-line opts
	opts, err := mongostat.ParseOptions(os.Args[1:], VersionStr, GitCommit)
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %s", err.Error())
		log.Logvf(log.Always, util.ShortUsage("mongostat"))
		os.Exit(util.ExitFailure)
	}

	log.SetVerbosity(opts.Verbosity)
	signals.Handle()

	// print help, if specified
	if opts.PrintHelp(false) {
		return
	}

	// print version, if specified
	if opts.PrintVersion() {
		return
	}

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	if opts.Auth.Username != "" && opts.GetAuthenticationDatabase() == "" &&
		!opts.Auth.RequiresExternalDB() {
		// add logic to have different error if using uri
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

	if opts.Interactive && opts.Json {
		log.Logvf(log.Always, "cannot use output formats --json and --interactive together")
		os.Exit(util.ExitFailure)
	}

	if opts.Deprecated && !opts.Json {
		log.Logvf(
			log.Always,
			"--useDeprecatedJsonKeys can only be used when --json is also specified",
		)
		os.Exit(util.ExitFailure)
	}

	if opts.Columns != "" && opts.AppendColumns != "" {
		log.Logvf(log.Always, "-O cannot be used if -o is also specified")
		os.Exit(util.ExitFailure)
	}

	if opts.HumanReadable != "true" && opts.HumanReadable != "false" {
		log.Logvf(log.Always, "--humanReadable must be set to either 'true' or 'false'")
		os.Exit(util.ExitFailure)
	}

	// we have to check this here, otherwise the user will be prompted
	// for a password for each discovered node
	if opts.Auth.ShouldAskForPassword() {
		pass, err := password.Prompt("mongo user")
		if err != nil {
			log.Logvf(log.Always, "Failed: %v", err)
			os.Exit(util.ExitFailure)
		}
		opts.Auth.Password = pass
	}

	var factory stat_consumer.FormatterConstructor
	if opts.Json {
		factory = stat_consumer.FormatterConstructors["json"]
	} else if opts.Interactive {
		factory = stat_consumer.FormatterConstructors["interactive"]
	} else {
		factory = stat_consumer.FormatterConstructors[""]
	}
	formatter := factory(opts.RowCount, !opts.NoHeaders)

	cliFlags := 0
	if opts.Columns == "" {
		cliFlags = line.FlagAlways
		if opts.Discover {
			cliFlags |= line.FlagDiscover
			cliFlags |= line.FlagHosts
		}
		if opts.All {
			cliFlags |= line.FlagAll
		}
		if strings.Contains(opts.Host, ",") {
			cliFlags |= line.FlagHosts
		}
	}

	var customHeaders []string
	if opts.Columns != "" {
		customHeaders = optionCustomHeaders(opts.Columns)
	} else if opts.AppendColumns != "" {
		customHeaders = optionCustomHeaders(opts.AppendColumns)
	}

	var keyNames map[string]string
	if opts.Deprecated {
		keyNames = line.DeprecatedKeyMap()
	} else if opts.Columns == "" {
		keyNames = line.DefaultKeyMap()
	} else {
		keyNames = optionKeyNames(opts.Columns)
	}
	if opts.AppendColumns != "" {
		addKN := optionKeyNames(opts.AppendColumns)
		for k, v := range addKN {
			keyNames[k] = v
		}
	}

	readerConfig := &status.ReaderConfig{
		HumanReadable: opts.HumanReadable == "true",
	}
	if opts.Json {
		readerConfig.TimeFormat = "15:04:05"
	}

	consumer := stat_consumer.NewStatConsumer(cliFlags, customHeaders,
		keyNames, readerConfig, formatter, os.Stdout)
	seedHosts := util.CreateConnectionAddrs(opts.Host, opts.Port)
	var cluster mongostat.ClusterMonitor
	if opts.Discover || len(seedHosts) > 1 {
		cluster = &mongostat.AsyncClusterMonitor{
			ReportChan:    make(chan *status.ServerStatus),
			ErrorChan:     make(chan *status.NodeError),
			LastStatLines: map[string]*line.StatLine{},
			Consumer:      consumer,
		}
	} else {
		cluster = &mongostat.SyncClusterMonitor{
			ReportChan: make(chan *status.ServerStatus),
			ErrorChan:  make(chan *status.NodeError),
			Consumer:   consumer,
		}
	}

	var discoverChan chan string
	if opts.Discover {
		discoverChan = make(chan string, 128)
	}

	opts.Direct = true
	stat := &mongostat.MongoStat{
		Options:       opts.ToolOptions,
		StatOptions:   opts.StatOptions,
		Nodes:         map[string]*mongostat.NodeMonitor{},
		Discovered:    discoverChan,
		SleepInterval: time.Duration(opts.SleepInterval) * time.Second,
		Cluster:       cluster,
	}

	for _, v := range seedHosts {
		if err := stat.AddNewNode(v); err != nil {
			log.Logv(log.Always, err.Error())
			os.Exit(util.ExitFailure)
		}
	}

	// kick it off
	err = stat.Run()
	for _, monitor := range stat.Nodes {
		monitor.Disconnect()
	}
	formatter.Finish()
	if err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitFailure)
	}
}
