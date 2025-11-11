package main

import (
	"fmt"
	"os"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/task-generator/cli"
	"github.com/mongodb/mongo-tools/mongodump_passthrough/task-generator/generate"
)

func main() {
	generate.InitForMongodumpTaskGen()
	cli.InitForMongodumpTaskGen()
	err := cli.App.Run(os.Args)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
