// Main package for the mongodump tool.
package main

import (
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongodump"
	"os"
	"time"
)

const progressBarWaitTime = time.Second * 3

func main() {
	// initialize command-line opts
	opts := options.New("mongodump", mongodump.Usage, options.EnabledOptions{true, true, true})

	inputOpts := &mongodump.InputOptions{}
	opts.AddOptions(inputOpts)
	outputOpts := &mongodump.OutputOptions{}
	opts.AddOptions(outputOpts)

	args, err := opts.Parse()
	if err != nil {
		log.Logf(log.Always, "error parsing command line options: %v", err)
		log.Logf(log.Always, "try 'mongodump --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	if len(args) > 0 {
		log.Logf(log.Always, "positional arguments not allowed: %v", args)
		log.Logf(log.Always, "try 'mongodump --help' for more information")
		os.Exit(util.ExitBadOptions)
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

	// connect directly, unless a replica set name is explicitly specified
	_, setName := util.ParseConnectionString(opts.Host)
	opts.Direct = (setName == "")
	opts.ReplicaSetName = setName

	
	dump := mongodump.MongoDump{
		ToolOptions:   opts,
		OutputOptions: outputOpts,
		InputOptions:  inputOpts,
		ProgressManager: progress.NewProgressBarManager(log.Writer(0), progressBarWaitTime),
		HandleSignals: mongodump.HandleSignals,
	}

	if err = dump.Init(); err != nil {
		log.Logf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitError)
	}

	if err = dump.Dump(); err != nil {
		log.Logf(log.Always, "Failed: %v", err)
		if err == util.ErrTerminated {
			os.Exit(util.ExitKill)
		}
		os.Exit(util.ExitError)
	}
}
