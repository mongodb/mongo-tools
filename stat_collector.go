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
)

// TruncateLength is the maximum number of characters allowed for long substrings when constructing
// log output lines.
const TruncateLength = 350

type StatOptions struct {
	Collect    string `long:"collect" description:"Stat collection format; 'format' option uses the --format string" choice:"json" choice:"format" choice:"none" default:"format"`
	Buffered   bool   `hidden:"yes"`
	Report     string `long:"report" description:"Write report on execution to given output path"`
	NoTruncate bool   `long:"no-truncate" description:"Disable truncation of large payload data in log output"`
	Format     string `long:"format" description:"Format for terminal output, %-escaped. Arguments are provided immediately after the escape, surrounded in curly braces. Supported escapes are:\n	%n namespace\n%l latency\n%t time (optional arg -- specify date layout, e.g. '%t{3:04PM}')\n%T op type\n%c command\n%o number of connections\n%i request ID\n%q request (optional arg -- dot-delimited field within the JSON structure, e.g. '%q{command_args.documents}')\n%r response (optional arg -- same as %q)\n%Q{<arg>} conditionally show <arg> on presence of request data\n%R{<arg>} conditionally show <arg> on presence of response data\nANSI escape sequences, start/end:\n%B/%b bold\n%U/%u underline\n%S/%s standout\n%F/%f text color (required arg -- word or number, 8-color)\n%K/%k background color (required arg -- same as %F/%f)\n" default:"%F{blue}%t%f %F{cyan}(Connection: %o:%i)%f %F{yellow}%l%f %F{red}%T %c%f %F{white}%n%f %F{green}%Q{Request:}%f%q %F{green}%R{Response:}%f%r"`
	NoColors   bool   `long:"no-colors" description:"Remove colors from the default format"`
}

// StatCollector is a struct that handles generation and recording of statistics about operations mongotape performs.
// It contains a StatGenerator and a StatRecorder that allow for differing implementations of the generating and recording functions
type StatCollector struct {
	sync.Once
	done       chan struct{}
	statStream chan *OpStat
	StatGenerator
	StatRecorder
	noop bool
}

func (statColl *StatCollector) Close() error {
	if statColl.noop || statColl.statStream == nil {
		return nil
	}
	statColl.StatGenerator.Finalize(statColl.statStream)
	close(statColl.statStream)
	<-statColl.done
	return statColl.StatRecorder.Close()
}

func newStatCollector(opts StatOptions, isPairedMode bool, isComparative bool) (*StatCollector, error) {
	if opts.Buffered {
		opts.Collect = "buffered"
	}
	if opts.Collect == "none" {
		return &StatCollector{noop: true}, nil
	}

	var statGen StatGenerator
	if isComparative {
		statGen = &ComparativeStatGenerator{}
	} else {
		statGen = &RegularStatGenerator{
			PairedMode:    isPairedMode,
			UnresolvedOps: make(map[opKey]UnresolvedOpInfo, 1024),
		}
	}

	if opts.NoColors {
		opts.Format = "%t (Connection: %o:%i) %l %T %c %n %Q{Request:}%q %R{Response:}%r"
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
	switch opts.Collect {
	case "json":
		statRec = &JSONStatRecorder{
			out: o,
		}
	case "buffered":
		statRec = &BufferedStatRecorder{
			Buffer: []OpStat{},
		}
	case "format":
		statRec = &TerminalStatRecorder{
			out:      o,
			truncate: !opts.NoTruncate,
			format:   opts.Format,
		}
	}

	return &StatCollector{
		StatGenerator: statGen,
		StatRecorder:  statRec,
	}, nil
}

// StatGenerator is an interface that specifies how to accept operation information to be recorded
type StatGenerator interface {
	GenerateOpStat(op *RecordedOp, replayedOp Op, reply replyContainer, msg string) *OpStat
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
	_, isReplyOp := op.(*ReplyOp)
	_, isCommandReplyOp := op.(*CommandReplyOp)
	return !isReplyOp && !isCommandReplyOp && !IsDriverOp(op)
}

// Collect formats the operation statistics as specified by the contained StatGenerator and writes it to
// some form of storage as specified by the contained StatRecorder
func (statColl *StatCollector) Collect(op *RecordedOp, replayedOp Op, reply replyContainer, msg string) {
	if statColl.noop {
		return
	}
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
	format   string
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
			toolDebugLogger.Logf(Always, "error recording stat: %v", err)
		}
		stat.RequestData = reqD
	}
	if stat.ReplyData != nil {
		repD, err := ConvertBSONValueToJSON(stat.ReplyData)
		if err != nil {
			toolDebugLogger.Logf(Always, "error recording stat: %v", err)
		}
		stat.ReplyData = repD
	}

	jsonBytes, err := json.Marshal(stat)
	if err != nil {
		toolDebugLogger.Logf(Always, "error recording stat: %v", err)
		return
	}
	_, err = jsr.out.Write(jsonBytes)
	if err != nil {
		toolDebugLogger.Logf(Always, "error recording stat: %v", err)
		return
	}
	_, err = jsr.out.Write([]byte("\n"))
	if err != nil {
		toolDebugLogger.Logf(Always, "error recording stat: %v", err)
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

	getPayload := func(data interface{}, out chan<- *bytes.Buffer) {
		if data == nil {
			out <- nil
			return
		}
		payload := new(bytes.Buffer)
		jsonData, err := ConvertBSONValueToJSON(data)
		if err != nil {
			toolDebugLogger.Logf(Always, "error recording stat: %v", err)
		}
		jsonBytes, err := json.Marshal(jsonData)
		if err != nil {
			payload.WriteString(err.Error())
		} else if dsr.truncate {
			payload.Write(AbbreviateBytes(jsonBytes, TruncateLength))
		} else {
			payload.Write(jsonBytes)
		}
		out <- payload
	}

	req := make(chan *bytes.Buffer)
	res := make(chan *bytes.Buffer)
	go getPayload(stat.RequestData, req)
	go getPayload(stat.ReplyData, res)

	output := new(bytes.Buffer)

	output.WriteString(stat.escaper(req, res).Expand(dsr.format))
	output.WriteString("\n")

	_, err := output.WriteTo(dsr.out)
	if err != nil {
		toolDebugLogger.Logf(Always, "error recording stat: %v", err)
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
	UnresolvedOps map[opKey]UnresolvedOpInfo
}

func (gen *ComparativeStatGenerator) GenerateOpStat(op *RecordedOp, replayedOp Op, reply replyContainer, msg string) *OpStat {
	if replayedOp == nil || op == nil {
		return nil
	}
	opMeta := replayedOp.Meta()
	stat := &OpStat{
		Order:         op.Order,
		OpType:        opMeta.Op,
		Ns:            opMeta.Ns,
		RequestData:   opMeta.Data,
		Command:       opMeta.Command,
		ConnectionNum: op.PlayedConnectionNum,
		Seen:          &op.Seen.Time,
		RequestId:     op.Header.RequestID,
	}
	var playAtHasVal bool
	if op.PlayAt != nil && !op.PlayAt.IsZero() {
		stat.PlayAt = &op.PlayAt.Time

		playAtHasVal = true
	}
	if op.PlayedAt != nil && !op.PlayedAt.IsZero() {
		stat.PlayedAt = &op.PlayedAt.Time

		if playAtHasVal {
			stat.PlaybackLagMicros = int64(op.PlayedAt.Sub(op.PlayAt.Time) / time.Microsecond)
		}
	}

	if reply.ReplyOp != nil {
		replyOp := reply.ReplyOp
		replyMeta := replyOp.Meta()

		stat.NumReturned = len(replyOp.Docs)
		stat.LatencyMicros = int64(reply.Latency / (time.Microsecond))
		stat.Errors = extractErrors(replayedOp, replyOp)
		stat.ReplyData = replyMeta.Data
	} else if reply.CommandReplyOp != nil {
		commandReplyOp := reply.CommandReplyOp
		crOpMeta := commandReplyOp.Meta()
		stat.NumReturned = len(commandReplyOp.Docs)
		stat.ReplyData = crOpMeta.Data
		stat.LatencyMicros = int64(reply.Latency / (time.Microsecond))
	} else {
		stat.ReplyData = nil
	}

	if msg != "" {
		stat.Message = msg
	}
	return stat
}

func (gen *RegularStatGenerator) GenerateOpStat(recordedOp *RecordedOp, parsedOp Op, reply replyContainer, msg string) *OpStat {
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
		ConnectionNum: recordedOp.SeenConnectionNum,
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
	for key, unresolved := range gen.UnresolvedOps {
		if gen.PairedMode {
			statStream <- unresolved.Stat
		}
		delete(gen.UnresolvedOps, key)
	}
	gen.UnresolvedOps = nil
}
