package mongoplay

import (
	"fmt"
	"io"
	"os"
	"time"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/10gen/mongoplay/mongoproto"
)

type PlayCommand struct {
	GlobalOpts   *Options `no-flag:"true"`
	PlaybackFile struct {
		PlaybackFile string
	} `required:"yes" positional-args:"yes" description:"The file to play back to the mongodb instance"`
	Url string `short:"m" long:"host" description:"Location of the host to play back against" default:"mongodb://localhost:27017"`
}

type SessionWrapper struct {
	session chan<- *OpWithTime
	done    <-chan bool
}

func newPlayOpChan(fileName string) (<-chan *OpWithTime, error) {
	opFile, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	ch := make(chan *OpWithTime)
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
			var doc OpWithTime
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

func newOpConnection(url string) (SessionWrapper, error) {
	session, err := mgo.Dial(url)
	if err != nil {
		return SessionWrapper{}, err
	}

	ch := make(chan *OpWithTime)
	done := make(chan bool)

	sessionWrapper := SessionWrapper{ch, done}
	go func() {
		for op := range ch {
			t := time.Now()
			if t.Before(op.PlayAt) {
				time.Sleep(op.PlayAt.Sub(t))
			}
			err = op.Execute(session)
			if err != nil {
				fmt.Printf("op.Execute error: %v\n", err)
			}
		}
		done<-true
	}()
	return sessionWrapper, nil
}

func (play *PlayCommand) Execute(args []string) error {
	fmt.Printf("%s", play.GlobalOpts.Verbose)
	//	play.Logger.Printf("%#v", play)
	opChan, err := newPlayOpChan(play.PlaybackFile.PlaybackFile)
	if err != nil {
		return fmt.Errorf("newPlayOpChan: %v", err)
	}
	var playbackStartTime, recordingStartTime time.Time
	var delta time.Duration
	sessionChans := make(map[string]SessionWrapper)

	for op := range opChan {
		if recordingStartTime.IsZero() && !op.Seen.IsZero() {
			recordingStartTime = op.Seen
			playbackStartTime = time.Now()
			delta = playbackStartTime.Sub(recordingStartTime)
		}
		// if we want to play faster or slower then delta will need to not be constant
		op.PlayAt = op.Seen.Add(delta)
		//fmt.Printf("play op %#v\n\n", op)
		sessionWrapper, ok := sessionChans[op.Connection]
		if !ok {
			sessionWrapper, err = newOpConnection(play.Url)
			if err != nil {
				fmt.Printf("newOpConnection: %v\n", err)
				os.Exit(1)
			}
			sessionChans[op.Connection] = sessionWrapper
		}
		sessionWrapper.session <- op
	}
	for _, sessionWrapper := range sessionChans {
		close(sessionWrapper.session)
	}
	for _, sessionWrapper := range sessionChans {
		<-sessionWrapper.done
	}
	return nil
}
