package main

import (
	"github.com/10gen/mongotape"
	"github.com/jessevdk/go-flags"

	"os"
)

const (
	ExitError    = 1
	ExitNonFatal = 3
	// Go reserves exit code 2 for its own use
)

func main() {
	opts := mongotape.Options{}
	var parser = flags.NewParser(&opts, flags.Default)
	parser.AddCommand("play", "Play captured traffic against a mongodb instance", "",
		&mongotape.PlayCommand{GlobalOpts: &opts})
	parser.AddCommand("record", "Convert network traffic into mongodb queries", "",
		&mongotape.RecordCommand{GlobalOpts: &opts})
	parser.AddCommand("monitor", "Inspect live or pre-recorded mongodb traffic", "",
		&mongotape.MonitorCommand{GlobalOpts: &opts})

	_, err := parser.Parse()
	if err != nil {
		switch err.(type) {
		case mongotape.ErrPacketsDropped:
			os.Exit(ExitNonFatal)
		default:
			os.Exit(ExitError)
		}
	}
}
