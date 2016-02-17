package main

import (
	"github.com/10gen/mongoplay"
	"github.com/jessevdk/go-flags"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"

	"os"
)

func main() {
	opts := mongoplay.Options{}
	var parser = flags.NewParser(&opts, flags.Default)
	parser.AddCommand("play", "Play captured traffic against a mongodb instance", "",
		&mongoplay.PlayCommand{GlobalOpts: &opts})
	parser.AddCommand("record", "Convert network traffic into mongodb queries", "",
		&mongoplay.RecordCommand{GlobalOpts: &opts})
	parser.AddCommand("stat", "Generate statistics on captured traffic", "",
		&mongoplay.StatCommand{GlobalOpts: &opts})
	// we want to default verbosity to 1 (info), so increment the default setting of 0
	opts.Verbose = append(opts.Verbose, true)
	log.SetVerbosity(&options.Verbosity{opts.Verbose, false})
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
