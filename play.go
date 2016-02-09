package mongoplay

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/10gen/llmgo/bson"
	"github.com/10gen/mongoplay/mongoproto"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
)

type PlayCommand struct {
	GlobalOpts   *Options `no-flag:"true"`
	PlaybackFile string   `description:"path to the playback file to play from" short:"p" long:"playback-file" required:"yes"`
	Speed        float64  `description:"multiplier for playback speed (1.0 = real-time, .5 = half-speed, 3.0 = triple-speed, etc.)" long:"speed" default:"1.0"`
	Url          string   `short:"h" long:"host" description:"Location of the host to play back against" default:"mongodb://localhost:27017"`
	Report       string   `long:"report" description:"Write report on execution to given output path"`
	Repeat       int      `long:"repeat" description:"Number of times to play the playback file" default:"1"`
	QueueTime    int      `long:"queueTime" description:"don't queue ops much further in the future than this number of seconds" default:"15"`
}

const queueGranularity = 1000

// NewPlayOpChan runs a goroutine that will read and unmarshal recorded ops
// from a file and push them in to a recorded op chan. Any errors encountered
// are pushed to an error chan. Both the recorded op chan and the error chan are
// returned by the function.
// The error chan won't be readable until the recorded op chan gets closed.
func (play *PlayCommand) NewPlayOpChan(file io.ReadSeeker) (<-chan *RecordedOp, <-chan error) {
	ch := make(chan *RecordedOp)
	e := make(chan error)

	var last time.Time
	var first time.Time
	var loopDelta time.Duration
	go func() {
		defer close(e)
		e <- func() error {
			defer close(ch)
			for generation := 0; generation < play.Repeat; generation++ {
				for {
					buf, err := mongoproto.ReadDocument(file)
					if err != nil {
						if err == io.EOF {
							break
						}
						return fmt.Errorf("ReadDocument: %v", err)
					}
					doc := &RecordedOp{}
					err = bson.Unmarshal(buf, doc)
					if err != nil {
						return fmt.Errorf("Unmarshal: %v\n", err)
					}
					last = doc.Seen
					if first.IsZero() {
						first = doc.Seen
					}
					doc.Seen = doc.Seen.Add(loopDelta)
					doc.Generation = generation
					// We want to suppress EOF's unless you're in the last generation
					// because all of the ops for one connection across different generations
					// get executed in the same session. We don't want to close the session
					// until the connection closes in the last generation.
					if !doc.EOF || generation == play.Repeat-1 {
						ch <- doc
					}
				}
				log.Logf(log.DebugHigh, "generation: %v", generation)
				loopDelta += last.Sub(first)
				first = time.Time{}
				_, err := file.Seek(0, 0)
				if err != nil {
					return fmt.Errorf("PlaybackFile Seek: %v", err)
				}
				continue
			}
			return io.EOF
		}()
	}()
	return ch, e
}

func openJSONReporter(path string) (*JSONStatCollector, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONStatCollector{out: f}, nil
}

func (play *PlayCommand) ValidateParams(args []string) error {
	switch {
	case len(args) > 0:
		return fmt.Errorf("unknown argument: %s", args[0])
	case play.Speed <= 0:
		return fmt.Errorf("Invalid setting for --speed: '%v'", play.Speed)
	case play.Repeat < 1:
		return fmt.Errorf("Invalid setting for --repeat: '%v', value must be >=1", play.Repeat)
	}
	return nil
}

func (play *PlayCommand) Execute(args []string) error {
	err := play.ValidateParams(args)
	if err != nil {
		return err
	}

	var statColl StatCollector = &NopCollector{}
	if len(play.Report) > 0 {
		statColl, err = openJSONReporter(play.Report)
		if err != nil {
			return err
		}
	}

	// we want to default verbosity to 1 (info), so increment the default setting of 0
	play.GlobalOpts.Verbose = append(play.GlobalOpts.Verbose, true)
	log.SetVerbosity(&options.Verbosity{play.GlobalOpts.Verbose, false})
	log.Logf(log.Info, "Doing playback at %.2fx speed", play.Speed)
	opFile, err := os.Open(play.PlaybackFile)
	if err != nil {
		return err
	}
	opChan, errChan := play.NewPlayOpChan(opFile)

	context := NewExecutionContext(statColl)
	if err := Play(context, opChan, play.Speed, play.Url, play.Repeat, play.QueueTime); err != nil {
		log.Logf(log.Always, "Play: %v\n", err)
	}

	//handle the error from the errchan
	err = <-errChan
	if err != io.EOF {
		log.Logf(log.Always, "OpChan: %v", err)
	}
	return nil
}

func Play(context *ExecutionContext,
	opChan <-chan *RecordedOp,
	speed float64,
	url string,
	repeat int,
	queueTime int) error {

	sessionChans := make(map[string]chan<- *RecordedOp)
	var playbackStartTime, recordingStartTime time.Time
	var connectionId int64
	var opCounter int
	for op := range opChan {
		opCounter++
		if op.Seen.IsZero() {
			return fmt.Errorf("Can't play operation found with zero-timestamp: %#v", op)
		}
		if recordingStartTime.IsZero() {
			recordingStartTime = op.Seen
			playbackStartTime = time.Now()
		}

		// opDelta is the difference in time between when the file's recording began and
		// and when this particular op is played. For the first operation in the playback, it's 0.
		opDelta := op.Seen.Sub(recordingStartTime)

		// Adjust the opDelta for playback by dividing it by playback speed setting;
		// e.g. 2x speed means the delta is half as long.
		scaledDelta := float64(opDelta) / (speed)
		op.PlayAt = playbackStartTime.Add(time.Duration(int64(scaledDelta)))

		// Every queueGranularity ops make sure that we're no more then QueueTime seconds ahead
		// Which should mean that the maximum that we're ever ahead
		// is QueueTime seconds of ops + queueGranularity more ops.
		// This is so that when we're at QueueTime ahead in the playback file we don't
		// sleep after every read, and generally read and queue queueGranularity number
		// of ops at a time and then sleep until the last read op is QueueTime ahead.
		if opCounter%queueGranularity == 0 {
			time.Sleep(op.PlayAt.Add(time.Duration(-queueTime) * time.Second).Sub(time.Now()))
		}

		var connectionString string
		if op.OpCode() == mongoproto.OpCodeReply {
			connectionString = op.ReversedConnectionString()
		} else {
			connectionString = op.ConnectionString()
		}
		sessionChan, ok := sessionChans[connectionString]
		if !ok {
			connectionId += 1
			sessionChan = context.newExecutionSession(url, op.PlayAt, connectionId)
			sessionChans[connectionString] = sessionChan
		}
		if op.EOF {
			close(sessionChan)
			delete(sessionChans, connectionString)
		} else {
			sessionChan <- op

		}
	}
	for connectionString, sessionChan := range sessionChans {
		close(sessionChan)
		delete(sessionChans, connectionString)
	}
	log.Logf(log.Info, "Waiting for sessions to finish")
	context.SessionChansWaitGroup.Wait()

	context.StatCollector.Close()
	log.Logf(log.Always, "%v ops played back in %v seconds over %v connections", opCounter, time.Now().Sub(playbackStartTime), connectionId)
	if repeat > 1 {
		log.Logf(log.Always, "%v ops per generation for %v generations", opCounter/repeat, repeat)
	}
	return nil
}
