package main

import (
	"fmt"
	"os"

	"github.com/mongodb/mongo-tools/dev-tools/mongodump-task-gen/generate"
)

func main() {
	if err := generate.New().WriteConfig(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
