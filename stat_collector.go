package mongotape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/10gen/llmgo/bson"
	"github.com/fatih/color"
)

// TruncateLength is the maximum number of characters allowed for long substrings when constructing
// log output lines.
const TruncateLength = 350

var (
	yellow  = color.New(color.FgYellow).SprintfFunc()
	red     = color.New(color.FgRed).SprintfFunc()
	green   = color.New(color.FgGreen).SprintfFunc()
	blue    = color.New(color.FgBlue).SprintfFunc()
	magenta = color.New(color.FgMagenta).SprintfFunc()
	cyan    = color.New(color.FgCyan).SprintfFunc()
	white   = color.New(color.FgWhite).SprintfFunc()
)

type StatOptions struct {
	JSON       bool   `long:"json" description:"Output operation data in JSON format"`
	Buffered   bool   `hidden:"yes"`
	Report     string `long:"report" description:"Write report on execution to given output path"`
	NoTruncate bool   `long:"no-truncate" description:"Disable truncation of large payload data in log output"`
	NoColors   bool   `long:"no-colors" description:"Disable colorized output"`
}

// StatCollector is a struct that handles generation and recording of statistics about operations mongotape performs.
// It contains a StatGenerator and a StatRecorder that allow for differing implementations of the generating and recording functions
type StatCollector struct {
	sync.Once
	done       chan struct{}
	statStream chan *OpStat
	StatGenerator
	StatRecorder
}

// OpStat is a set of metadata about an executed operation and its result which can be
// used for generating reports about the results of a playback command.
type OpStat struct {
	// Order is a number denoting the position in the traffic in which this operation appeared
	Order int64 `json:"order"`

	// OpType is a string representation of the function of this operation. For example an 'insert'
	// or a 'query'
	OpType string `json:"op,omitempty"`

	// If the operation was a command, this field represents the name of the database command
	// performed, like "isMaster" or "getLastError". Left blank for ops that are not commands
	// like a query, insert, or getmore.
	Command string `json:"command,omitempty"`

	// Namespace that the operation was performed against, if relevant.
	Ns string `json:"ns,omitempty"`

	// Data represents the payload of the request operation.
	RequestData interface{} `json:"request_data, omitempty"`

	// Data represents the payload of the reply operation.
	ReplyData interface{} `json:"reply_data, omitempty"`

	// NumReturned is the number of documents that were fetched as a result of this operation.
	NumReturned int `json:"nreturned,omitempty"`

	// PlayedAt is the time that this operation was replayed
	PlayedAt *time.Time `json:"played_at,omitempty"`

	// PlayAt is the time that this operation is scheduled to be played. It represents the time
	// that it is supposed to be played by mongotape, but can be different from
	// PlayedAt if the playback is lagging for any reason
	PlayAt *time.Time `json:"play_at,omitempty"`

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
	LatencyMicros int64 `json:"latency_us,omitempty"`

	// Errors contains the error messages returned from the server populated in the $err field.
	// If unset, the operation did not receive any errors from the server.
	Errors []string `json:"errors,omitempty"`

	Message string `json:"msg,omitempty"`

	// Seen is the time that this operation was originally seen.
	Seen *time.Time `json:"seen,omitempty"`

	// RequestId is the Id of the mongodb operation as taken from the header.
	// The RequestId for a request operation is the same as the ResponseId for
	// the corresponding reply, so this field will be the same for request/reply pairs.
	RequestId int32 `json:"request_id, omitempty"`
}

func (statColl *StatCollector) Close() error {
	if statColl.statStream == nil {
		return nil
	}
	statColl.StatGenerator.Finalize(statColl.statStream)
	close(statColl.statStream)
	<-statColl.done
	return statColl.StatRecorder.Close()
}

func newStatCollector(opts StatOptions, isPairedMode bool, isComparative bool) (*StatCollector, error) {

	if opts.NoColors {
		color.NoColor = true
	}
	var statGen StatGenerator
	if isComparative {
		statGen = &ComparativeStatGenerator{}
	} else {
		statGen = &RegularStatGenerator{
			PairedMode:    isPairedMode,
			UnresolvedOps: make(map[string]UnresolvedOpInfo, 1024),
		}
	}

	var o io.WriteCloser
	var err error
	if opts.Report != "" {
		o, err = os.Create(opts.Report)
		if err != nil {
			return nil, err
		}
	} else {
		o = os.Stdout
	}

	var statRec StatRecorder
	switch {
	case opts.JSON:
		statRec = &JSONStatRecorder{
			out: o,
		}
	case opts.Buffered:
		statRec = &BufferedStatRecorder{
			Buffer: []OpStat{},
		}
	default:
		statRec = &TerminalStatRecorder{
			out:      o,
			truncate: !opts.NoTruncate,
		}
	}

	return &StatCollector{
		StatGenerator: statGen,
		StatRecorder:  statRec,
	}, nil
}

// StatGenerator is an interface that specifies how to accept operation information to be recorded
type StatGenerator interface {
	GenerateOpStat(op *RecordedOp, replayedOp Op, reply *ReplyOp, msg string) *OpStat
	Finalize(chan *OpStat)
}

// StatRecorder is an interface that specifies how to take OpStats to be recorded
type StatRecorder interface {
	RecordStat(stat *OpStat)
	Close() error
}

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
func extractErrors(op Op, reply *ReplyOp) []string {
	if len(reply.Docs) == 0 {
		return nil
	}

	retVal := []string{}
	firstDoc := bson.D{}
	err := reply.Docs[0].Unmarshal(&firstDoc)
	if err != nil {
		panic("failed to unmarshal Raw into bson.D")
	}
	if val, ok := FindValueByKey("$err", &firstDoc); ok {
		retVal = append(retVal, fmt.Sprintf("%v", val))
	}

	if qop, ok := op.(*QueryOp); ok {
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
func shouldCollectOp(op Op) bool {
	_, ok := op.(*ReplyOp)
	return !ok && !IsDriverOp(op)
}

// Collect formats the operation statistics as specified by the contained StatGenerator and writes it to
// some form of storage as specified by the contained StatRecorder
func (statColl *StatCollector) Collect(op *RecordedOp, replayedOp Op, reply *ReplyOp, msg string) {
	statColl.Do(func() {
		statColl.statStream = make(chan *OpStat, 1024)
		statColl.done = make(chan struct{})
		go func() {
			for stat := range statColl.statStream {
				statColl.StatRecorder.RecordStat(stat)
			}
			close(statColl.done)
		}()
	})
	if stat := statColl.GenerateOpStat(op, replayedOp, reply, msg); stat != nil {
		statColl.statStream <- stat
	}
}

type JSONStatRecorder struct {
	out io.WriteCloser
}

type TerminalStatRecorder struct {
	out      io.WriteCloser
	truncate bool
}

// BufferedStatRecorder implements the StatRecorder interface using an in-memory slice of OpStats.
// This allows for the statistics on operations executed by mongotape to be reviewed by a program directly following execution.
// BufferedStatCollector's main purpose is for asserting correct execution of ops for testing
type BufferedStatRecorder struct {
	// Buffer is a slice of OpStats that is appended to every time the Collect function makes a record
	// It stores an in-order series of OpStats that store information about the commands mongotape ran as a result
	// of reading a playback file
	Buffer []OpStat
}
type NopRecorder struct{}

func (jsr *JSONStatRecorder) RecordStat(stat *OpStat) {
	if stat == nil {
		// TODO log warning.
		return
	}

	// TODO use variant of this function that does not mutate its argument.
	if stat.RequestData != nil {
		reqD, err := ConvertBSONValueToJSON(stat.RequestData)
		if err != nil {
			// TODO log a warning.
		}
		stat.RequestData = reqD
	}
	if stat.ReplyData != nil {
		repD, err := ConvertBSONValueToJSON(stat.ReplyData)
		if err != nil {
			// TODO log a warning.
		}
		stat.ReplyData = repD
	}

	jsonBytes, err := json.Marshal(stat)
	if err != nil {
		// TODO log error?
		return
	}
	_, err = jsr.out.Write(jsonBytes)
	if err != nil {
		// TODO log error?
		return
	}
	_, err = jsr.out.Write([]byte("\n"))
	if err != nil {
		// TODO log error?
		return
	}
}

func (bsr *BufferedStatRecorder) RecordStat(stat *OpStat) {
	bsr.Buffer = append(bsr.Buffer, *stat)
}

func (dsr *TerminalStatRecorder) RecordStat(stat *OpStat) {
	if stat == nil {
		// TODO log warning.
		return
	}

	var payload bytes.Buffer
	if stat.RequestData != nil {
		reqD, err := ConvertBSONValueToJSON(stat.RequestData)
		if err != nil {
			// TODO log a warning.
		}
		stat.RequestData = reqD
		payload.WriteString(green("Request:"))
		jsonBytes, err := json.Marshal(stat.RequestData)
		if err != nil {
			payload.WriteString(err.Error())
		} else {
			if dsr.truncate {
				payload.WriteString(Abbreviate(string(jsonBytes), TruncateLength))
			} else {
				payload.Write(jsonBytes)
			}
			payload.WriteString(" ")
		}
	}
	if stat.ReplyData != nil {
		repD, err := ConvertBSONValueToJSON(stat.ReplyData)
		if err != nil {
			// TODO log a warning.
		}
		stat.ReplyData = repD
		stat.RequestData = repD
		payload.WriteString(green("Reply:"))
		jsonBytes, err := json.Marshal(stat.ReplyData)
		if err != nil {
			payload.WriteString(err.Error())
		} else {
			if dsr.truncate {
				payload.WriteString(Abbreviate(string(jsonBytes), TruncateLength))
			} else {
				payload.Write(jsonBytes)
			}
			payload.WriteString(" ")
		}
	}

	var output bytes.Buffer

	var timestamp *time.Time = stat.Seen
	if stat.PlayedAt != nil {
		timestamp = stat.PlayedAt
	}
	output.WriteString(blue("%v", timestamp.Format("2/15 15:04:05.000")))

	output.WriteString(cyan(" (Connection %v:%v)", stat.ConnectionNum, stat.RequestId))

	if stat.LatencyMicros > 0 {
		latency := time.Microsecond * time.Duration(stat.LatencyMicros)
		output.WriteString(yellow(fmt.Sprintf(" +%v", latency.String())))
	}
	if stat.OpType != "" {
		output.WriteString(red(" %v", stat.OpType))
	}
	if stat.Ns != "" {
		output.WriteString(white(" %v", stat.Ns))
	}

	output.WriteString(" ")
	payload.WriteTo(&output)

	output.WriteString("\n")

	_, err := output.WriteTo(dsr.out)
	if err != nil {
		// TODO log error?
		return
	}
}

func (nr *NopRecorder) RecordStat(stat *OpStat) {
}

func (jsr *JSONStatRecorder) Close() error {
	return jsr.out.Close()
}

func (bsr *BufferedStatRecorder) Close() error {
	return nil
}

func (nc *NopRecorder) Close() error {
	return nil
}
func (dsr *TerminalStatRecorder) Close() error {
	return dsr.out.Close()
}

type ComparativeStatGenerator struct {
}

type RegularStatGenerator struct {
	PairedMode    bool
	UnresolvedOps map[string]UnresolvedOpInfo
}

func (gen *ComparativeStatGenerator) GenerateOpStat(op *RecordedOp, replayedOp Op, reply *ReplyOp, msg string) *OpStat {
	if replayedOp == nil || op == nil {
		return nil
	}
	opMeta := replayedOp.Meta()
	stat := &OpStat{
		Order:             op.Order,
		OpType:            opMeta.Op,
		Ns:                opMeta.Ns,
		RequestData:       opMeta.Data,
		Command:           opMeta.Command,
		ConnectionNum:     op.ConnectionNum,
		PlaybackLagMicros: int64(op.PlayedAt.Sub(op.PlayAt.Time) / time.Microsecond),
		Seen:              &op.Seen.Time,
	}
	if !op.PlayAt.IsZero() {
		stat.PlayAt = &op.PlayAt.Time
	}
	if !op.PlayedAt.IsZero() {
		stat.PlayedAt = &op.PlayedAt.Time
	}
	if reply != nil {
		replyMeta := reply.Meta()
		stat.NumReturned = len(reply.Docs)
		stat.LatencyMicros = int64(reply.Latency / (time.Microsecond))
		stat.Errors = extractErrors(replayedOp, reply)
		stat.ReplyData = replyMeta.Data
	} else {
		stat.ReplyData = nil
	}
	if msg != "" {
		stat.Message = msg
	}
	return stat
}

func (gen *RegularStatGenerator) GenerateOpStat(recordedOp *RecordedOp, parsedOp Op, reply *ReplyOp, msg string) *OpStat {
	if recordedOp == nil || parsedOp == nil {
		// TODO log a warning
		return nil
	}
	meta := parsedOp.Meta()
	stat := &OpStat{
		Order:         recordedOp.Order,
		OpType:        meta.Op,
		Ns:            meta.Ns,
		Command:       meta.Command,
		ConnectionNum: recordedOp.ConnectionNum,
		Seen:          &recordedOp.Seen.Time,
	}
	if msg != "" {
		stat.Message = msg
	}
	switch recordedOp.Header.OpCode {
	case OpCodeQuery, OpCodeGetMore:
		stat.RequestData = meta.Data
		stat.RequestId = recordedOp.Header.RequestID
		gen.AddUnresolvedOp(recordedOp, parsedOp, stat)
		// In 'PairedMode', the stat is not considered completed at this point.
		// We save the op as 'unresolved' and return nil. When the reply is seen
		// we retrieve the saved stat and generate a completed pair stat, which
		// is then returned.
		if gen.PairedMode {
			return nil
		}
	case OpCodeReply:
		stat.RequestId = recordedOp.Header.ResponseTo
		stat.ReplyData = meta.Data
		return gen.ResolveOp(recordedOp, parsedOp.(*ReplyOp), stat)
	default:
		stat.RequestData = meta.Data
	}
	return stat
}

func (gen *ComparativeStatGenerator) Finalize(statStream chan *OpStat) {}

func (gen *RegularStatGenerator) Finalize(statStream chan *OpStat) {
	for _, unresolved := range gen.UnresolvedOps {
		statStream <- unresolved.Stat
	}
}
