package main

import (
	"fmt"
	"log"
	"os"

	"github.com/10gen/mongoplay"
)

func usage() string {
	return fmt.Sprintf("usage:\n\t%v (play|record) <args>", os.Args[0])
}

func main() {
	var err error
	logger := log.New(os.Stdout, "mongoplay: ", 0)
	if len(os.Args) < 2 {
		logger.Fatal(usage())
	}
	command := os.Args[1]
	switch command {
	case "record":
		r := &mongoplay.RecordConf{
			Logger: logger,
		}
		err = r.ParseRecordFlags(os.Args[2:])
		if err != nil {
			logger.Fatal(err)
		}
		err = r.Record()
		if err != nil {
			logger.Fatal(err)
		}
	case "play":
		p := &mongoplay.PlayConf{
			Logger: logger,
		}
		err = p.ParsePlayFlags(os.Args[2:])
		if err != nil {
			logger.Fatal(err)
		}
		err = p.Play()
		if err != nil {
			logger.Fatal(err)
		}
	default:
		logger.Fatal(usage())

	}
}
