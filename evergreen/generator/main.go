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

	pr, err := c.GitHubPRAliasesYAML()
	if err != nil {
		panic(err)
	}

	mq, err := c.MergeQueueAliasesYAML()
	if err != nil {
		panic(err)
	}

	fmt.Println(pr)
	fmt.Println(mq)
}
