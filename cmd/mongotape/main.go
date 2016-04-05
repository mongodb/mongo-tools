package main

import (
	"github.com/10gen/mongotape"
	"github.com/jessevdk/go-flags"

	"os"
)

func main() {
	opts := mongotape.Options{}
	var parser = flags.NewParser(&opts, flags.Default)
	parser.AddCommand("play", "Play captured traffic against a mongodb instance", "",
		&mongotape.PlayCommand{GlobalOpts: &opts})
	parser.AddCommand("record", "Convert network traffic into mongodb queries", "",
		&mongotape.RecordCommand{GlobalOpts: &opts})
	parser.AddCommand("stat", "Generate statistics on captured traffic", "",
		&mongotape.StatCommand{GlobalOpts: &opts})

	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
