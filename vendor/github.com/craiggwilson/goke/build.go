package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/craiggwilson/goke/build"
	"github.com/craiggwilson/goke/task"
)

func main() {
	err := task.Run(build.Registry(), os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(1)
		}
		fmt.Println(err)
		os.Exit(2)
	}
}
