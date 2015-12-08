package main

import (
	"flag"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"math/rand"
	"os"
	"time"
)

func genLoad(host string, quitChan, finishChan chan int) {
	session, err := mgo.Dial(host)
	if err != nil {
		fmt.Printf("error dialing host (%v) : %v\n", host, err)
		os.Exit(1)
	}
	defer session.Close()
	db := session.DB("foo")
	for {
		var i int
		select {
		case i = <-quitChan:
			fmt.Printf("finishing %v\n", i)
			finishChan <- i
			return
		default:
		}
		c := db.C(fmt.Sprintf("col%v", rand.Intn(32)))
		r := rand.Intn(100)
		switch {
		//case r < 25: // update
		case r < 50: // insert
			c.Insert(bson.M{"Id": rand.Intn(4096), "Num": 0})
		default: // find
			result := bson.M{}
			iter := c.Find(bson.M{"Id": rand.Intn(4096)}).Iter()
			for iter.Next(result) {
			}
		}

	}
}

func main() {
	duration := flag.Int("duration", 60, "how long to run the load generator")
	connections := flag.Int("connections", 1024, "how many connections to use")
	host := flag.String("host", "localhost:27017", "host to connect to")
	flag.Parse()
	finishChan := make(chan int, 1)
	quitChan := make(chan int, 1)
	for i := 0; i < *connections; i++ {
		go genLoad(*host, quitChan, finishChan)
	}
	time.Sleep(time.Duration(*duration) * time.Second)
	go func() {
		for i := 0; i < *connections; i++ {
			fmt.Printf("killing %v\n", i)
			quitChan <- i
		}
	}()
	for i := 0; i < *connections; i++ {
		finished := <-finishChan
		fmt.Printf("dead %v\n", finished)
	}
}
