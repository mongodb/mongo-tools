package mongoplay

import (
	"fmt"
	"github.com/10gen/mongoplay/mongoproto"
	"io"
	"time"
)

type StatCommand struct {
	GlobalOpts   *Options `no-flag:"true"`
	PlaybackFile string   `description:"path to the playback file to analyze for stats" short:"p" long:"playback-file" required:"yes"`
	Report       string   `long:"report" description:"Write report on execution to given output path" required:"yes"`
}

type UnresolvedOpInfo struct {
	Stat     *OpStat
	ParsedOp mongoproto.Op
	Op       *RecordedOp
}

//AddUnresolved takes an operation that is supposed to receive a reply and keeps it around so that its latency can be calculated
//using the incoming reply
func (gen *StaticStatGenerator) AddUnresolvedOp(op *RecordedOp, parsedOp mongoproto.Op, stat *OpStat) {
	key := fmt.Sprintf("%v:%v:%d", op.SrcEndpoint, op.DstEndpoint, op.Header.RequestID)
	unresolvedOp := UnresolvedOpInfo{
		Stat:     stat,
		Op:       op,
		ParsedOp: parsedOp,
	}
	gen.UnresolvedOps[key] = unresolvedOp
}

func (gen *StaticStatGenerator) ResolveOp(recordedReply *RecordedOp, parsedReply *mongoproto.ReplyOp) *OpStat {
	key := fmt.Sprintf("%v:%v:%d", recordedReply.DstEndpoint, recordedReply.SrcEndpoint, recordedReply.Header.ResponseTo)
	originalOpInfo, ok := gen.UnresolvedOps[key]
	if !ok {
		return nil
	}

	originalOpInfo.Stat.LatencyMicros = int64(recordedReply.Seen.Sub(originalOpInfo.Op.Seen) / (time.Microsecond))
	originalOpInfo.Stat.Errors = extractErrors(originalOpInfo.ParsedOp, parsedReply)
	originalOpInfo.Stat.NumReturned = int(parsedReply.ReplyDocs)
	delete(gen.UnresolvedOps, key)
	return originalOpInfo.Stat
}

func (stat *StatCommand) Execute(args []string) error {
	playbackFileReader, err := NewPlaybackFileReader(stat.PlaybackFile)
	if err != nil {
		return err
	}

	jsonStatRecorder, err := openJSONRecorder(stat.Report)
	if err != nil {
		return err
	}
	staticStatGenerator := &StaticStatGenerator{
		UnresolvedOps: make(map[string]UnresolvedOpInfo, 1024),
	}

	statColl := &StatCollector{
		StatGenerator: staticStatGenerator,
		StatRecorder:  jsonStatRecorder,
	}

	var order int64 = 0
	for {
		recordedOp, err := playbackFileReader.NextRecordedOp()
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		recordedOp.Order = order
		parsedOp, err := recordedOp.OpRaw.Parse()
		if err != nil {
			return err
		}
		if !mongoproto.IsDriverOp(parsedOp) {
			statColl.Collect(recordedOp, parsedOp, nil, "")
		}
		order++
	}
	statColl.Close()
	return nil
}
