package main

import (
	"fmt"
	"log"
	"os"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
)

func main() {
	mgo.SetDebug(true)
	mgo.SetLogger(log.New(os.Stderr, "", log.LstdFlags))
	session, err := mgo.Dial("localhost")
	if err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}
	var r bson.M

	data, reply, err := session.ExecOpWithReply(&mgo.QueryOp{Collection: "test.bar", Limit: 0})
	if err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}
	fmt.Printf("%v, %#v\n", data, reply)
	for _, d := range data {
		err = bson.Unmarshal(d, &r)
		if err != nil {
			fmt.Printf("%v", err)
			os.Exit(1)
		}
		fmt.Printf("%#v\n", r)
	}

	data2, reply2, err2 := session.ExecOpWithReply(&mgo.GetMoreOp{Collection: "test.bar", Limit: 0, CursorId: reply.CursorId})
	if err2 != nil {
		fmt.Printf("%v", err2)
		os.Exit(1)
	}
	fmt.Printf("GetMoreOp %v, %#v\n", data2, reply2)
	for _, d := range data2 {
		err = bson.Unmarshal(d, &r)
		if err != nil {
			fmt.Printf("%v", err)
			os.Exit(1)
		}
		fmt.Printf("getmore %#v\n", r)
	}

	err3 := session.ExecOpWithoutReply(&mgo.KillCursorsOp{[]int64{reply.CursorId}})
	if err3 != nil {
		fmt.Printf("%v", err2)
		os.Exit(1)
	}
}
