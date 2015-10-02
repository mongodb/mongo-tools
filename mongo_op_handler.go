package mongocaputils

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"container/heap"

	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"

	"github.com/tmc/mongocaputils/mongoproto"
	"github.com/tmc/mongocaputils/tcpreaderwrapper"
)

type MongoOpStream struct {
	Ops chan OpWithTime

	FirstSeen    time.Time
	unorderedOps chan OpWithTime
	opHeap       *orderedOps

	mu sync.Mutex // for debugging
}

func NewMongoOpStream(heapBufSize int) *MongoOpStream {
	h := make(orderedOps, 0, heapBufSize)
	s := &MongoOpStream{
		Ops:          make(chan OpWithTime), // ordered
		unorderedOps: make(chan OpWithTime), // unordered
		opHeap:       &h,
	}
	heap.Init(s.opHeap)
	go s.handleOps()
	return s
}

func (s *MongoOpStream) New(a, b gopacket.Flow) tcpassembly.Stream {
	r := tcpreaderwrapper.NewReaderStreamWrapper()
	log.Println("starting stream", a, b)
	go s.handleStream(&r)
	return &r
}

func (s *MongoOpStream) Close() error {
	close(s.unorderedOps)
	s.unorderedOps = nil
	return nil
}

func (s *MongoOpStream) SetFirstSeen(t time.Time) {
	s.FirstSeen = t
}

func (s *MongoOpStream) handleOps() {
	defer close(s.Ops)
	for op := range s.unorderedOps {
		heap.Push(s.opHeap, op)
		if len(*s.opHeap) == cap(*s.opHeap) {
			s.Ops <- heap.Pop(s.opHeap).(OpWithTime)
		}
	}
	for len(*s.opHeap) > 0 {
		s.Ops <- heap.Pop(s.opHeap).(OpWithTime)
	}
}

func (s *MongoOpStream) readOp(r io.Reader) (mongoproto.Op, error) {
	return mongoproto.OpFromReader(r)
}

func (s *MongoOpStream) handleStream(r *tcpreaderwrapper.ReaderStreamWrapper) {
	lastSeen := s.FirstSeen
	for {
		op, err := s.readOp(r)
		if err == io.EOF {
			discarded, err := ioutil.ReadAll(r)
			if len(discarded) != 0 || err != nil {
				fmt.Println("discarded ", len(discarded), err)
			}
			return
		}
		if err == tcpreader.DataLost {
			log.Println("ignoring incomplete packet")
			discarded, err := ioutil.ReadAll(r)
			if len(discarded) != 0 || err != nil {
				fmt.Println("discarded ", len(discarded), err)
			}
			return
		}
		if err != nil {
			log.Println("error parsing op:", err)
			discarded, err := ioutil.ReadAll(r)
			if len(discarded) != 0 || err != nil {
				fmt.Println("discarded ", len(discarded), err)
			}
			return
		}
		seen := lastSeen
		if len(r.Reassemblies) > 0 {
			seen = r.Reassemblies[0].Seen
			lastSeen = seen
		}
		for _, r := range r.Reassemblies {
			if r.NumBytes > 0 {
				seen = r.Seen
			}
		}
		s.unorderedOps <- OpWithTime{op, seen}
		r.Reassemblies = r.Reassemblies[:0]
	}
}
