// Main package for the mongodump tool.
package main

import (
	"os"
	"time"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/signals"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongodump"
)

const (
	progressBarLength   = 24
	progressBarWaitTime = time.Second * 3
)

func main() {
	// initialize command-line opts
	opts := options.New("mongodump", mongodump.Usage, options.EnabledOptions{Auth: true, Connection: true, Namespace: true, URI: true})

	inputOpts := &mongodump.InputOptions{}
	opts.AddOptions(inputOpts)
	outputOpts := &mongodump.OutputOptions{}
	opts.AddOptions(outputOpts)
	opts.URI.AddKnownURIParameters(options.KnownURIOptionsReadPreference)

	args, err := opts.ParseArgs(os.Args[1:])
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %v", err)
		log.Logvf(log.Always, "try 'mongodump --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	if len(args) > 0 {
		log.Logvf(log.Always, "positional arguments not allowed: %v", args)
		log.Logvf(log.Always, "try 'mongodump --help' for more information")
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

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	// kick off the progress bar manager
	progressManager := progress.NewBarWriter(log.Writer(0), progressBarWaitTime, progressBarLength, false)
	progressManager.Start()
	defer progressManager.Stop()

	dump := mongodump.MongoDump{
		ToolOptions:     opts,
		OutputOptions:   outputOpts,
		InputOptions:    inputOpts,
		ProgressManager: progressManager,
	}

	finishedChan := signals.HandleWithInterrupt(dump.HandleInterrupt)
	defer close(finishedChan)

	if err = dump.Init(); err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitError)
	}

	if err = dump.Dump(); err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		if err == util.ErrTerminated {
			os.Exit(util.ExitKill)
		}
		os.Exit(util.ExitError)
	}
}
