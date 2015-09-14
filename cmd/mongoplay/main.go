package main

import (
	"flag"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"os"
	"time"

	"github.com/gabrielrussell/mongocaputils"
	"github.com/gabrielrussell/mongocaputils/mongoproto"
)

var (
	messageFile   = flag.String("f", "-", "message file (or '-' for stdin)")
	packetBufSize = flag.Int("size", 1000, "size of packet buffer used for ordering within streams")
	verbose       = flag.Bool("v", false, "verbose output (to stderr)")
	host          = flag.String("h", "127.0.0.1:27017", "mongod host (127.0.0.1:27017)")
)

func newPlayOpChan(fileName string) (<-chan *mongocaputils.OpWithTime, error) {
	opFile, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	ch := make(chan *mongocaputils.OpWithTime)
	go func() {
		defer close(ch)
		for {
			buf, err := mongoproto.ReadDocument(opFile)
			if err != nil {
				fmt.Printf("ReadDocument: %v\n", err)
				if err == io.EOF {
					return
				}
				os.Exit(1)
			}
			var doc mongocaputils.OpWithTime
			err = bson.Unmarshal(buf, &doc)
			if err != nil {
				fmt.Printf("Unmarshal: %v\n", err)
				os.Exit(1)
			}
			ch <- &doc

		}
	}()
	return ch, nil
}

func newOpConnection(url string) (chan<- *mongocaputils.OpWithTime, error) {
	session, err := mgo.Dial(url)
	if err != nil {
		return nil, err
	}
	ch := make(chan *mongocaputils.OpWithTime)
	go func() {
		for op := range ch {
			t := time.Now()
			if t.Before(op.PlayAt) {
				time.Sleep(op.PlayAt.Sub(t))
			}
			err = op.Execute(session)
			if err != nil {
				fmt.Printf("op.Execute %v", err)
			}
		}
	}()
	return ch, nil
}

func main() {
	flag.Parse()
	opChan, err := newPlayOpChan(*messageFile)
	if err != nil {
		fmt.Printf("newPlayOpChan: %v\n", err)
		os.Exit(1)
	}
	var playbackStartTime, recordingStartTime time.Time
	var delta time.Duration
	sessions := make(map[string]chan<- *mongocaputils.OpWithTime)
	for op := range opChan {
		if recordingStartTime.IsZero() && !op.Seen.IsZero() {
			recordingStartTime = op.Seen
			playbackStartTime = time.Now()
			delta = playbackStartTime.Sub(recordingStartTime)
		}
		// if we want to play faster or slower then delta will need to not be constant
		op.PlayAt = op.Seen.Add(delta)
		fmt.Printf("%#v\n\n", op)
		session, ok := sessions[op.Connection]
		if !ok {
			session, err = newOpConnection(*host)
			if err != nil {
				fmt.Printf("newOpConnection: %v\n", err)
				os.Exit(1)
			}
			sessions[op.Connection] = session
		}
		session <- op
	}
}
