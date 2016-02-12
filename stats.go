package mongoplay

import (
	"encoding/json"
	"fmt"
	"github.com/10gen/llmgo/bson"
	"github.com/10gen/mongoplay/mongoproto"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"
)

// FindValueByKey returns the value of keyName in document.
// The second return arg is a bool which is true if and only if the key was present in the doc.
func FindValueByKey(keyName string, document *bson.D) (interface{}, bool) {
	for _, key := range *document {
		if key.Name == keyName {
			return key.Value, true
		}
	}
	return nil, false
}

// extractErrors inspects a request/reply pair and returns all the error messages that were found.
func extractErrors(op mongoproto.Op, res *mongoproto.OpResult) []string {
	if len(res.Docs) == 0 {
		return nil
	}

	retVal := []string{}
	firstDoc := bson.D{}
	err := res.Docs[0].Unmarshal(&firstDoc)
	if err != nil {
		panic("failed to unmarshal Raw into bson.D")
	}
	if val, ok := FindValueByKey("$err", &firstDoc); ok {
		retVal = append(retVal, fmt.Sprintf("%v", val))
	}

	if qop, ok := op.(*mongoproto.QueryOp); ok {
		if strings.HasSuffix(qop.Collection, "$cmd") {
			// This query was actually a command, so we should look for errors in the following
			// places in the returned document:
			// - the "$err" field, which is set if bit #1 is set on the responseFlags
			// - the "errmsg" field on the top-level returned document
			// - the "writeErrors" and "writeConcernErrors" arrays, which contain objects
			//   that each have an "errmsg" field

			// TODO if more than one doc was returned by the query,
			// something weird is going on, and we should print a warning.
			if val, ok := FindValueByKey("errmsg", &firstDoc); ok {
				retVal = append(retVal, fmt.Sprintf("%v", val))
			}

			if val, ok := FindValueByKey("writeErrors", &firstDoc); ok {
				switch reflect.TypeOf(val).Kind() {
				case reflect.Slice:
					s := reflect.ValueOf(val)
					for i := 0; i < s.Len(); i++ {
						retVal = append(retVal, fmt.Sprintf("%v", s.Index(i).Interface()))
					}
				}
			}

			if val, ok := FindValueByKey("writeConcernErrors", &firstDoc); ok {
				switch reflect.TypeOf(val).Kind() {
				case reflect.Slice:
					s := reflect.ValueOf(val)
					for i := 0; i < s.Len(); i++ {
						retVal = append(retVal, fmt.Sprintf("%v", s.Index(i).Interface()))
					}
				}
			}

		}
	}
	return retVal
}

// OpStat is a set of metadata about an executed operation and its result which can be
// used for generating reports about the results of a playback command.
type OpStat struct {
	Order  int64  `json:"order"`
	OpType string `json:"op,omitempty"`

	// If the operation was a command, this field represents the name of the database command
	// performed, like "isMaster" or "getLastError". Left blank for ops that are not commands
	// like a query, insert, or getmore.
	Command string `json:"command,omitempty"`

	// Namespace that the operation was performed against, if relevant.
	Ns string `json:"ns,omitempty"`

	NumReturned int `json:"nreturned,omitempty"`

	PlayedAt time.Time `json:"played_at"`
	PlayAt   time.Time `json:"play_at"`

	// PlaybackLagMicros is the time difference in microseconds between the time
	// that the operation was supposed to be played, and the time it was actualy played.
	// High values indicate that playback is falling behind the intended rate.
	PlaybackLagMicros int64 `json:"playbacklag_us,omitempty"`

	// ConnectionNum represents the number of the connection that the op originated from.
	// This number does not correspond to any server-side connection IDs - it's simply an
	// auto-incrementing number representing the order in which the connection was created
	// during the playback phase.
	ConnectionNum int64 `json:"connection_num"`

	// LatencyMicros represents the time difference in microseconds between when the operation
	// was executed and when the reply from the server was received.
	LatencyMicros int64 `json:"latency_us"`

	// Errors contains the error messages returned from the server populated in the $err field.
	// If unset, the operation did not receive any errors from the server.
	Errors []string `json:"errors,omitempty"`
}

func GenerateOpStat(op *RecordedOp, replayedOp mongoproto.Op, res *mongoproto.OpResult) OpStat {
	opMeta := replayedOp.Meta()
	stat := OpStat{
		OpType:            opMeta.Op,
		Ns:                opMeta.Ns,
		Command:           opMeta.Command,
		ConnectionNum:     op.ConnectionNum,
		PlaybackLagMicros: int64(op.PlayedAt.Sub(op.PlayAt) / time.Microsecond),
		PlayAt:            op.PlayAt,
		PlayedAt:          op.PlayedAt,
	}
	if res != nil {
		stat.NumReturned = len(res.Docs)
		stat.LatencyMicros = int64(res.Latency / (time.Microsecond))
		stat.Errors = extractErrors(replayedOp, res)
	}

	return stat
}

type StatCollector interface {
	CollectOpInfo(op *RecordedOp, replayedOp mongoproto.Op, res *mongoproto.OpResult)
	Close() error
}

//BufferedStatCollector implements the StatCollector interface using an in-memory slice of OpStats.
//This allows for the statistics on operations executed by mongoplay to be reviewed by a program directly following execution.
//BufferedStatCollector's main purpose is for asserting correct execution of ops for testing
type BufferedStatCollector struct {
	startup    sync.Once
	statStream chan statInfo
	done       chan struct{}
	//Buffer is a slice of OpStats that is appended to every time CollectOpInfo is called in mongoplay
	//It stores an in-order series of OpStats that store information about the commands mongoplay ran as a result
	//of reading a playback file
	Buffer []OpStat
}

type JSONStatCollector struct {
	startup    sync.Once
	statStream chan statInfo
	done       chan struct{}
	out        io.WriteCloser
}

type statInfo struct {
	op         *RecordedOp
	replayedOp mongoproto.Op
	res        *mongoproto.OpResult
}

type NopCollector struct{}

func (nc *NopCollector) CollectOpInfo(op *RecordedOp, replayedOp mongoproto.Op, res *mongoproto.OpResult) {
}
func (nc *NopCollector) Close() error {
	return nil
}

func (jsc *JSONStatCollector) CollectOpInfo(op *RecordedOp, replayedOp mongoproto.Op, res *mongoproto.OpResult) {
	// Ensure that the goroutine for processing stats is started the first time (and *only* the
	// first time) this func is called.
	jsc.startup.Do(func() {
		jsc.statStream = make(chan statInfo, 1024)
		jsc.done = make(chan struct{})
		order := int64(0)
		go func() {
			for item := range jsc.statStream {
				order++
				stat := GenerateOpStat(item.op, item.replayedOp, item.res)
				stat.Order = order
				jsonBytes, err := json.Marshal(stat)
				if err != nil {
					// TODO log error?
					continue
				}

				_, err = jsc.out.Write(jsonBytes)
				if err != nil {
					// TODO log error?
					continue
				}
				_, err = jsc.out.Write([]byte("\n"))
				if err != nil {
					// TODO log error?
					continue
				}
			}
			close(jsc.done)
		}()
	})
	jsc.statStream <- statInfo{op, replayedOp, res}
}
func (jsc *JSONStatCollector) Close() error {
	close(jsc.statStream)
	_ = <-jsc.done
	return jsc.out.Close()
}

func NewBufferedStatCollector() *BufferedStatCollector {
	return &BufferedStatCollector{
		Buffer: []OpStat{},
	}
}

func (bsc *BufferedStatCollector) CollectOpInfo(op *RecordedOp, replayedOp mongoproto.Op, res *mongoproto.OpResult) {
	// Ensure that the goroutine for processing stats is started the first time (and *only* the
	// first time) this func is called.
	bsc.startup.Do(func() {
		bsc.statStream = make(chan statInfo, 1024)
		bsc.done = make(chan struct{})
		order := int64(0)
		go func() {
			for item := range bsc.statStream {
				order++
				stat := GenerateOpStat(item.op, item.replayedOp, item.res)
				stat.Order = order
				bsc.Buffer = append(bsc.Buffer, stat)
			}
			close(bsc.done)
		}()
	})
	bsc.statStream <- statInfo{op, replayedOp, res}
}

func (bsc *BufferedStatCollector) Close() error {
	close(bsc.statStream)
	<-bsc.done
	return nil
}
