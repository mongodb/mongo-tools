package main

import (
	"fmt"

	"github.com/mongodb/mongo-tools/evergreen"
)

func main() {
	c, err := evergreen.Load()
	if err != nil {
		panic(err)
	}

	y, err := c.GitHubPRAliasesYAML()
	if err != nil {
		panic(err)
	}

	fmt.Println(y)
}
