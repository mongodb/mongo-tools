package mongoplay

import (
	"container/heap"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"

	"github.com/mongodb/mongo-tools/common/log"

	"github.com/10gen/mongoplay/mongoproto"
)

// tcpassembly.Stream implementation.

type stream struct {
	bidi             *bidi
	reassembled      chan []tcpassembly.Reassembly
	reassembly       tcpassembly.Reassembly
	done             chan interface{}
	op               *mongoproto.RawOp
	state            streamState
	netFlow, tcpFlow gopacket.Flow
}

// Reassembled receives the new slice of reassembled data and forwards it to the
// MongoOpStream->streamOps goroutine for which turns them in to protocol
// messages.
// Since the tcpassembler reuses the tcpreassembly.Reassembled buffers, we
// wait for streamOps to signal us that it's done with them before returning.
func (stream *stream) Reassembled(reassembly []tcpassembly.Reassembly) {
	stream.reassembled <- reassembly
	<-stream.done
}

// ReassemblyComplete receives from the tcpassembler the fact that the stream is
// now finished. Because our streamOps function may be reading from more then
// one stream, we only shut down the bidi once all the streams are finished.
func (stream *stream) ReassemblyComplete() {
	count := atomic.AddInt32(&stream.bidi.openStreamCount, -1)
	if count < 0 {
		panic("negative openStreamCount")
	}
	if count == 0 {
		stream.bidi.close()
	}
}

// tcpassembly.StreamFactory implementation.

// bidi is a bidirectional connection.
type bidi struct {
	streams          [2]*stream
	openStreamCount  int32
	opStream         *MongoOpStream
	responseStream   bool
	sawStart         bool
	connectionNumber int
}

func newBidi(netFlow, tcpFlow gopacket.Flow, opStream *MongoOpStream) *bidi {
	bidi := &bidi{}
	bidi.streams[0] = &stream{
		bidi:        bidi,
		reassembled: make(chan []tcpassembly.Reassembly),
		done:        make(chan interface{}),
		op:          &mongoproto.RawOp{},
		netFlow:     netFlow,
		tcpFlow:     tcpFlow,
	}
	bidi.streams[1] = &stream{
		bidi:        bidi,
		reassembled: make(chan []tcpassembly.Reassembly),
		done:        make(chan interface{}),
		op:          &mongoproto.RawOp{},
		netFlow:     netFlow.Reverse(),
		tcpFlow:     tcpFlow.Reverse(),
	}
	bidi.opStream = opStream
	bidi.connectionNumber = opStream.connectionNumber
	opStream.connectionNumber++
	return bidi
}

func (bidi *bidi) logf(minVerb int, format string, a ...interface{}) {
	log.Logf(minVerb, "stream %v %v", bidi.connectionNumber, fmt.Sprintf(format, a...))
}

// close closes the channels used to communicate between the
// stream's and bidi.streamOps,
// and removes the bidi from the bidiMap.
func (bidi *bidi) close() {
	close(bidi.streams[0].reassembled)
	close(bidi.streams[0].done)
	close(bidi.streams[1].reassembled)
	close(bidi.streams[1].done)
	delete(bidi.opStream.bidiMap, bidiKey{bidi.streams[1].netFlow, bidi.streams[1].tcpFlow})
	delete(bidi.opStream.bidiMap, bidiKey{bidi.streams[0].netFlow, bidi.streams[0].tcpFlow})
	// probably not important, just trying to make the garbage collection easier.
	bidi.streams[0].bidi = nil
	bidi.streams[1].bidi = nil
}

type bidiKey struct {
	net, transport gopacket.Flow
}

type MongoOpStream struct {
	Ops chan RecordedOp

	FirstSeen        time.Time
	unorderedOps     chan RecordedOp
	opHeap           *orderedOps
	bidiMap          map[bidiKey]*bidi
	connectionNumber int
}

func NewMongoOpStream(heapBufSize int) *MongoOpStream {
	h := make(orderedOps, 0, heapBufSize)
	os := &MongoOpStream{
		Ops:          make(chan RecordedOp), // ordered
		unorderedOps: make(chan RecordedOp), // unordered
		opHeap:       &h,
		bidiMap:      make(map[bidiKey]*bidi),
	}
	heap.Init(os.opHeap)
	go os.handleOps()
	return os
}

// New is the factory method called by the tcpassembly to generate new tcpassembly.Stream.
func (os *MongoOpStream) New(netFlow, tcpFlow gopacket.Flow) tcpassembly.Stream {
	key := bidiKey{netFlow, tcpFlow}
	rkey := bidiKey{netFlow, tcpFlow}
	if bidi, ok := os.bidiMap[key]; ok {
		atomic.AddInt32(&bidi.openStreamCount, 1)
		delete(os.bidiMap, key)
		return bidi.streams[1]
	} else {
		bidi := newBidi(netFlow, tcpFlow, os)
		os.bidiMap[rkey] = bidi
		atomic.AddInt32(&bidi.openStreamCount, 1)
		go bidi.streamOps()
		return bidi.streams[0]
	}
}

// Close is called by the tcpassembly to indicate that all of the packets
// have been processed.
func (os *MongoOpStream) Close() error {
	close(os.unorderedOps)
	os.unorderedOps = nil
	return nil
}

// All of this SetFirstSeen/FirstSeen/SetFirstseer stuff
// can go away ( from here and from packet_handler.go )
// it's a cruft and was how someone was trying to get around
// the fact that using the tcpassembly.tcpreader library
// throws away all of the metadata about the stream.
func (os *MongoOpStream) SetFirstSeen(t time.Time) {
	os.FirstSeen = t
}

// handleOps runs all of the ops read from the
// unorderedOps through a heapsort and then runs
// them out on the Ops channel.
func (os *MongoOpStream) handleOps() {
	defer close(os.Ops)
	for op := range os.unorderedOps {
		heap.Push(os.opHeap, op)
		if len(*os.opHeap) == cap(*os.opHeap) {
			os.Ops <- heap.Pop(os.opHeap).(RecordedOp)
		}
	}
	for len(*os.opHeap) > 0 {
		os.Ops <- heap.Pop(os.opHeap).(RecordedOp)
	}
}

type streamState int

func (st streamState) String() string {
	switch st {
	case streamStateBeforeMessage:
		return "Before Message"
	case streamStateInMessage:
		return "In Message"
	case streamStateOutOfSync:
		return "Out Of Sync"
	}
	return "Unknown"
}

const (
	streamStateBeforeMessage streamState = iota
	streamStateInMessage
	streamStateOutOfSync
)

func (bidi *bidi) handleStreamStateBeforeMessage(stream *stream) {
	if stream.reassembly.Start {
		if bidi.sawStart {
			panic("apparently saw the beginning of a connection twice")
		}
		bidi.sawStart = true
	}
	// TODO deal with the situation that the first packet doesn't contain a whole MessageHeader
	// of an otherwise valid protocol message.
	// The following code erroneously assumes that all packets will have at least 16 bytes of data
	if len(stream.reassembly.Bytes) < 16 {
		stream.state = streamStateOutOfSync
		stream.reassembly.Bytes = stream.reassembly.Bytes[:0]
		return
	}
	stream.op.Header.FromWire(stream.reassembly.Bytes)
	if !stream.op.Header.LooksReal() {
		// When we're here and stream.reassembly.Start is true
		// we may be able to know that we're actually not looking at mongodb traffic
		// and that this whole stream should be discarded.
		bidi.logf(log.DebugLow, "not a good header %#v", stream.op.Header)
		bidi.logf(log.Info, "Expected to, but didn't see a valid protocol message")
		stream.state = streamStateOutOfSync
		stream.reassembly.Bytes = stream.reassembly.Bytes[:0]
		return
	}
	stream.op.Body = make([]byte, 16, stream.op.Header.MessageLength)
	stream.state = streamStateInMessage
	copy(stream.op.Body, stream.reassembly.Bytes)
	stream.reassembly.Bytes = stream.reassembly.Bytes[16:]
	return
}
func (bidi *bidi) handleStreamStateInMessage(stream *stream) {
	var copySize int
	bodyLen := len(stream.op.Body)
	if bodyLen+len(stream.reassembly.Bytes) > int(stream.op.Header.MessageLength) {
		copySize = int(stream.op.Header.MessageLength) - bodyLen
	} else {
		copySize = len(stream.reassembly.Bytes)
	}
	stream.op.Body = stream.op.Body[:bodyLen+copySize]
	copied := copy(stream.op.Body[bodyLen:], stream.reassembly.Bytes)
	if copied != copySize {
		panic("copied != copySize")
	}
	stream.reassembly.Bytes = stream.reassembly.Bytes[copySize:]
	if len(stream.op.Body) == int(stream.op.Header.MessageLength) {
		//TODO maybe remember if we were recently in streamStateOutOfSync,
		// and if so, parse the raw op here.
		bidi.opStream.unorderedOps <- RecordedOp{RawOp: *stream.op, Seen: stream.reassembly.Seen, SrcEndpoint: stream.netFlow.Src().String(), DstEndpoint: stream.netFlow.Dst().String()}
		stream.op = &mongoproto.RawOp{}
		stream.state = streamStateBeforeMessage
		if len(stream.reassembly.Bytes) > 0 {
			// parse the remainder of the stream.reassembly as a new message.
			return
		}
	}
	return
}
func (bidi *bidi) handleStreamStateOutOfSync(stream *stream) {
	bidi.logf(log.DebugHigh, "out of sync")
	if len(stream.reassembly.Bytes) < 16 {
		stream.reassembly.Bytes = stream.reassembly.Bytes[:0]
		return
	}
	stream.op.Header.FromWire(stream.reassembly.Bytes)
	bidi.logf(log.DebugHigh, "possible message header %#v", stream.op.Header)
	if stream.op.Header.LooksReal() {
		stream.state = streamStateBeforeMessage
		bidi.logf(log.DebugLow, "synchronized")
		return
	}
	stream.reassembly.Bytes = stream.reassembly.Bytes[:0]
	return
}

// streamOps reads tcpassembly.Reassembly[] blocks from the
// stream's and tries to create whole protocol messages from them.
func (bidi *bidi) streamOps() {
	bidi.logf(log.Info, "starting")
	for {
		var reassemblies []tcpassembly.Reassembly
		var reassembliesStream int
		var ok bool
		select {
		case reassemblies, ok = <-bidi.streams[0].reassembled:
			reassembliesStream = 0
		case reassemblies, ok = <-bidi.streams[1].reassembled:
			reassembliesStream = 1
		}
		if !ok {
			break
		}
		stream := bidi.streams[reassembliesStream]

		for _, stream.reassembly = range reassemblies {
			// Skip > 0 means that we've missed something, and we have incomplete packets in hand.
			if stream.reassembly.Skip > 0 {
				// TODO, we may want to do more state specific reporting here.
				stream.state = streamStateOutOfSync
				stream.op.Body = stream.op.Body[:0]
				bidi.logf(log.Info, "ignoring incomplete packet")
				continue
			}
			// Skip < 0 means that we're picking up a stream mid-stream, and we don't really know the
			// state of what's in hand;
			// we need to synchronize.
			if stream.reassembly.Skip < 0 {
				bidi.logf(log.Info, "capture started in the middle a stream")
				stream.state = streamStateOutOfSync
			}

			for len(stream.reassembly.Bytes) > 0 {
				bidi.logf(log.DebugHigh, "state %v", stream.state)
				switch stream.state {
				case streamStateBeforeMessage:
					bidi.handleStreamStateBeforeMessage(stream)
				case streamStateInMessage:
					bidi.handleStreamStateInMessage(stream)
				case streamStateOutOfSync:
					bidi.handleStreamStateOutOfSync(stream)
				}
			}
		}
		// inform the tcpassembly that we've finished with the reassemblies.
		stream.done <- nil
	}
	bidi.logf(log.Info, "finishing")
}
