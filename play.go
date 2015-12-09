package mongoplay

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"io"
	"os"
	"time"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/10gen/mongoplay/mongoproto"
)

type PlayCommand struct {
	GlobalOpts   *Options `no-flag:"true"`
	PlaybackFile string   `description:"path to the playback file to play from" short:"p" long:"playback-file" required:"yes"`
	Speed        float64  `description:"multiplier for playback speed (1.0 = real-time, .5 = half-speed, 3.0 = triple-speed, etc.)" long:"speed" default:"1.0"`
	Url          string   `short:"h" long:"host" description:"Location of the host to play back against" default:"mongodb://localhost:27017"`
}

type SessionWrapper struct {
	session chan<- *RecordedOp
	done    <-chan bool
}

func NewPlayOpChan(fileName string) (<-chan *RecordedOp, error) {
	opFile, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	ch := make(chan *RecordedOp)
	go func() {
		defer close(ch)
		for {
			buf, err := mongoproto.ReadDocument(opFile)
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Logf(log.Always, "Error calling ReadDocument: %v\n", err)
				os.Exit(1)
			}
			var doc RecordedOp
			err = bson.Unmarshal(buf, &doc)
			if err != nil {
				log.Logf(log.Always, "Error calling Unmarshal: %v\n", err)
				os.Exit(1)
			}
			ch <- &doc
		}
	}()
	return ch, nil
}

func newOpConnection(url string, context *ExecutionContext, connectionId int64) (SessionWrapper, error) {
	session, err := mgo.Dial(url)
	if err != nil {
		return SessionWrapper{}, err
	}

	ch := make(chan *RecordedOp, 10000)
	done := make(chan bool)

	sessionWrapper := SessionWrapper{ch, done}
	go func() {
		log.Logf(log.Info, "(Connection %v) New connection CREATED.", connectionId)
		for op := range ch {
			t := time.Now()
			if t.Before(op.PlayAt) {
				time.Sleep(op.PlayAt.Sub(t))
			}
			err = context.Execute(op, session, connectionId)
			if err != nil {
				log.Logf(log.Always, "context.Execute error: %v", err)
			}
		}
		log.Logf(log.Info, "(Connection %v) Connection ENDED.", connectionId)
		done <- true
	}()
	return sessionWrapper, nil
}

func (play *PlayCommand) Execute(args []string) error {
	if play.Speed <= 0 {
		return fmt.Errorf("Invalid setting for --speed: '%v'", play.Speed)
	}

	// we want to default verbosity to 1 (info), so increment the default setting of 0
	play.GlobalOpts.Verbose = append(play.GlobalOpts.Verbose, true)
	log.SetVerbosity(&options.Verbosity{play.GlobalOpts.Verbose, false})
	log.Logf(log.Info, "Doing playback at %.2fx speed", play.Speed)
	opChan, err := NewPlayOpChan(play.PlaybackFile)
	if err != nil {
		return fmt.Errorf("newPlayOpChan: %v", err)
	}
	var playbackStartTime, recordingStartTime time.Time

	// recordVsPlaybackDelta represents the difference in time between when
	// the file was recorded and the time that we begin playing it back.
	//var recordVsPlaybackDelta time.Duration
	sessionChans := make(map[string]SessionWrapper)

	context := ExecutionContext{
		IncompleteReplies: map[string]ReplyPair{},
		CompleteReplies:   map[string]ReplyPair{},
		CursorIDMap:       map[int64]int64{},
	}

	var connectionId int64
	for op := range opChan {
		if op.Seen.IsZero() {
			return fmt.Errorf("Can't play operation found with zero-timestamp: %#v", op)
		}
		if recordingStartTime.IsZero() && !op.Seen.IsZero() {
			recordingStartTime = op.Seen
			playbackStartTime = time.Now()
		}

		// opDelta is the difference in time between when the file's recording began and
		// and when this particular op is played. For the first operation in the playback, it's 0.
		opDelta := op.Seen.Sub(recordingStartTime)

		// Adjust the opDelta for playback by dividing it by playback speed setting;
		// e.g. 2x speed means the delta is half as long.
		scaledDelta := float64(opDelta) / (play.Speed)
		op.PlayAt = playbackStartTime.Add(time.Duration(int64(scaledDelta)))

		var connectionString string
		if op.OpCode() == mongoproto.OpCodeReply {
			connectionString = op.ReversedConnectionString()
		} else {
			connectionString = op.ConnectionString()
		}
		sessionWrapper, ok := sessionChans[connectionString]
		if !ok {
			connectionId += 1
			sessionWrapper, err = newOpConnection(play.Url, &context, connectionId)
			if err != nil {
				log.Logf(log.Always, "Error calling newOpConnection: %v", err)
				os.Exit(1)
			}
			sessionChans[connectionString] = sessionWrapper
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
