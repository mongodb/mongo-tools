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

	"github.com/gabrielrussell/mongocaputils/mongoproto"
	"github.com/gabrielrussell/mongocaputils/tcpreader"
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
	r := tcpreader.NewReaderStream()
	log.Println("starting stream", a, b)
	go s.handleStream(&r, b.String())
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

func (s *MongoOpStream) readOpRaw(r io.Reader) (*mongoproto.OpRaw, time.Time, error) {
	return mongoproto.OpRawFromReader(r)
}

func (s *MongoOpStream) handleStream(r *tcpreader.ReaderStream, connection string) {
	for {
		op, seen, err := s.readOpRaw(r)
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
		s.unorderedOps <- OpWithTime{OpRaw: *op, Seen: seen, Connection: connection}
	}
}
