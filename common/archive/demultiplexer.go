package archive

import (
	"bytes"
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"gopkg.in/mgo.v2/bson"
	"hash"
	"hash/crc64"
	"io"
)

// DemuxOut is a Demultiplexer output consumer
// The Write() and Close() occur in the same thread as the Demultiplexer runs in.
type DemuxOut interface {
	Write([]byte) (int, error)
	Close() error
}

// Demultiplexer implements Parser.
type Demultiplexer struct {
	In io.Reader
	//TODO wrap up these three into a structure
	outs               map[string]DemuxOut
	hashes             map[string]hash.Hash64
	lengths            map[string]int64
	currentNamespace   string
	buf                [db.MaxBSONSize]byte
	NamespaceChan      chan string
	NamespaceErrorChan chan error
}

// Run creates and runs a parser with the Demultiplexer as a consumer
func (demux *Demultiplexer) Run() error {
	parser := Parser{In: demux.In}
	err := parser.ReadAllBlocks(demux)
	if len(demux.outs) > 0 {
		log.Logf(log.Always, "demux finishing when there are still outs (%v)", len(demux.outs))
	}
	log.Logf(log.DebugLow, "demux finishing (err:%v)", err)
	return err
}

type demuxError struct {
	Err error
	Msg string
}

// Error is part of the Error interface. It formats a demuxError for human readability.
func (pe *demuxError) Error() string {
	err := fmt.Sprintf("error demultiplexing archive; %v", pe.Msg)
	if pe.Err != nil {
		err = fmt.Sprintf("%v ( %v )", err, pe.Err)
	}
	return err
}

// newError creates a demuxError with just a message
func newError(msg string) error {
	return &demuxError{
		Msg: msg,
	}
}

// newWrappedError creates a demuxError with a message as well as an underlying cause error
func newWrappedError(msg string, err error) error {
	return &demuxError{
		Err: err,
		Msg: msg,
	}
}

// HeaderBSON is part of the ParserConsumer interface and receives headers from parser.
// Its main role is to implement opens and EOFs of the embedded stream.
func (demux *Demultiplexer) HeaderBSON(buf []byte) error {
	colHeader := NamespaceHeader{}
	err := bson.Unmarshal(buf, &colHeader)
	if err != nil {
		return newWrappedError("header bson doesn't unmarshal as a collection header", err)
	}
	log.Logf(log.DebugHigh, "demux namespaceHeader: %v", colHeader)
	if colHeader.Database == "" {
		return newError("collection header is missing a Database")
	}
	if colHeader.Collection == "" {
		return newError("collection header is missing a Collection")
	}
	demux.currentNamespace = colHeader.Database + "." + colHeader.Collection
	if _, ok := demux.outs[demux.currentNamespace]; !ok {
		if demux.NamespaceChan != nil {
			demux.NamespaceChan <- demux.currentNamespace
			err := <-demux.NamespaceErrorChan
			if err != nil {
				return newWrappedError("failed arranging a consumer for new namespace", err)
			}
		}
	}
	if colHeader.EOF {
		crc := int64(demux.hashes[demux.currentNamespace].Sum64())
		length := int64(demux.lengths[demux.currentNamespace])
		if crc != colHeader.CRC {
			return fmt.Errorf("CRC mismatch for namespace %v, %v!=%v",
				demux.currentNamespace,
				crc,
				colHeader.CRC,
			)
		}
		log.Logf(log.DebugHigh, "demux checksum for namespace %v is correct (%v). lenght: %v",
			demux.currentNamespace, crc, length)
		demux.outs[demux.currentNamespace].Close()
		delete(demux.outs, demux.currentNamespace)
		delete(demux.hashes, demux.currentNamespace)
		delete(demux.lengths, demux.currentNamespace)
		// in case we get a BSONBody with this block, we want to ensure that that causes an error
		demux.currentNamespace = ""
	}
	return nil
}

// End is part of the ParserConsumer interface and receives the end of archive notification.
func (demux *Demultiplexer) End() error {
	log.Logf(log.DebugHigh, "demux End")
	if len(demux.outs) != 0 {
		openNss := []string{}
		for ns := range demux.outs {
			openNss = append(openNss, ns)
		}
		return newError(fmt.Sprintf("archive finished but contained files were unfinished (%v)", openNss))
	}

	if demux.NamespaceChan != nil {
		close(demux.NamespaceChan)
	}
	return nil
}

// BodyBSON is part of the ParserConsumer interface and receives BSON bodies from the parser.
// Its main role is to dispatch the body to the Read() function of the current DemuxOut.
func (demux *Demultiplexer) BodyBSON(buf []byte) error {
	if demux.currentNamespace == "" {
		return newError("collection data without a collection header")
	}
	hash, ok := demux.hashes[demux.currentNamespace]
	if !ok {
		return newError("no checksum for current namespace " + demux.currentNamespace)
	}
	hash.Write(buf)

	demux.lengths[demux.currentNamespace] += int64(len(buf))

	out, ok := demux.outs[demux.currentNamespace]
	if !ok {
		return newError("no demux consumer currently consuming namespace " + demux.currentNamespace)
	}
	_, err := out.Write(buf)
	return err
}

// Open installs the DemuxOut as the handler for data for the namespace ns
func (demux *Demultiplexer) Open(ns string, out DemuxOut) {
	// In the current implementation where this is either called before the demultiplexing is running
	// or while the demutiplexer is inside of the NamespaceChan NamespaceErrorChan conversation
	// I think that we don't need to lock outs, but I suspect that if the implementation changes
	// we may need to lock when outs is accessed
	log.Logf(log.DebugHigh, "demux Open")
	if demux.outs == nil {
		demux.outs = make(map[string]DemuxOut)
		demux.hashes = make(map[string]hash.Hash64)
		demux.lengths = make(map[string]int64)
	}
	demux.outs[ns] = out
	demux.hashes[ns] = crc64.New(crc64.MakeTable(crc64.ECMA))
	demux.lengths[ns] = 0
}

// RegularCollectionReceiver implements the intents.file interface.
// RegularCollectionReceivers get paired with RegularCollectionSenders.
type RegularCollectionReceiver struct {
	readLenChan      <-chan int
	readBufChan      chan<- []byte
	Intent           *intents.Intent
	Demux            *Demultiplexer
	partialReadArray [db.MaxBSONSize]byte
	partialReadBuf   []byte
	isOpen           bool
}

func (receiver *RegularCollectionReceiver) Read(r []byte) (int, error) {
	if receiver.partialReadBuf != nil && len(receiver.partialReadBuf) > 0 {
		wLen := len(receiver.partialReadBuf)
		copyLen := copy(r, receiver.partialReadBuf)
		if wLen == copyLen {
			receiver.partialReadBuf = nil
		} else {
			receiver.partialReadBuf = receiver.partialReadBuf[copyLen:]
		}
		return copyLen, nil
	}
	// Since we're the "reader" here, not the "writer" we need to start with a read, in case the chan is closed
	wLen, ok := <-receiver.readLenChan
	if !ok {
		close(receiver.readBufChan)
		return 0, io.EOF
	}
	if wLen > db.MaxBSONSize {
		return 0, fmt.Errorf("incomming buffer size is too big %v", wLen)
	}
	rLen := len(r)
	if wLen > rLen {
		// if the incomming write size is larger then the incomming read buffer then we need to accept
		// the write in a larger buffer, fill the read buffer, then cache the remainder
		receiver.partialReadBuf = receiver.partialReadArray[:wLen]
		receiver.readBufChan <- receiver.partialReadBuf
		writtenLength := <-receiver.readLenChan
		if wLen != writtenLength {
			return 0, fmt.Errorf("regularCollectionSender didn't send what it said it would")
		}
		copy(r, receiver.partialReadBuf)
		receiver.partialReadBuf = receiver.partialReadBuf[rLen:]
		return rLen, nil
	}
	// Send the read buff to the BodyBSON ParserConsumer to fill
	receiver.readBufChan <- r
	// Receiver the wLen of data written
	wLen = <-receiver.readLenChan
	return wLen, nil
}

// Close is part of the intents.file interface. It currently does nothing. We can't close the
// regularCollectionSender before the embedded stream reaches EOF. If this needs to be
// implemented, then we need to swap out the regularCollectionSender with a null writer
func (receiver *RegularCollectionReceiver) Close() error {
	return nil
}

// Open is part of the intents.file interface.  It creates the chan's in the
// RegularCollectionReceiver and adds the RegularCollectionReceiver to the set of
// RegularCollectonReceivers in the demultiplexer
func (receiver *RegularCollectionReceiver) Open() error {
	// TODO move this implementation to some non intents.file method, to be called from prioritizer.Get
	// So that we don't have to enable this double open stuff.
	// Currently the open needs to finish before the prioritizer.Get finishes, so we open the intents.file
	// in prioritizer.Get even though it's going to get opened again in DumpIntent.
	if receiver.isOpen {
		return nil
	}
	readLenChan := make(chan int)
	readBufChan := make(chan []byte)
	receiver.readLenChan = readLenChan
	receiver.readBufChan = readBufChan
	sender := &regularCollectionSender{readLenChan: readLenChan, readBufChan: readBufChan}
	receiver.Demux.Open(receiver.Intent.Namespace(), sender)
	receiver.isOpen = true
	return nil
}

// Write is part of the intents.file interface.
// It does nothing, and only exists so that RegularCollectionReceiver fulfills the interface
func (receiver *RegularCollectionReceiver) Write([]byte) (int, error) {
	return 0, nil
}

// regularCollectionSender implements DemuxOut
type regularCollectionSender struct {
	readLenChan chan<- int
	readBufChan <-chan []byte
}

// Write is part of the DemuxOut interface.
func (sender *regularCollectionSender) Write(buf []byte) (int, error) {
	//  As a writer, we need to write first, so that the reader can properly detect EOF
	//  Additionally, the reader needs to know the write size, so that it can give us a
	//  properly sized buffer. Sending the incomming buffersize fills both of these needs.
	sender.readLenChan <- len(buf)
	// Receive from the reader a buffer to put the bytes into
	readBuf := <-sender.readBufChan
	if len(readBuf) < len(buf) {
		return 0, fmt.Errorf("readbuf is not large enough for incoming BodyBSON (%v<%v)",
			len(readBuf), len(buf))
	}
	copy(readBuf, buf)
	// Send back the length of the data copied in to the buffer
	sender.readLenChan <- len(buf)
	return len(buf), nil
}

// Close is part of the DemuxOut interface. It only closes the readLenChan, as that is what will
// cause the RegularCollectionReceiver.Read() to receive EOF
func (sender *regularCollectionSender) Close() error {
	close(sender.readLenChan)
	return nil
}

// SpecialCollectionCache implemnts both DemuxOut as well as intents.file
type SpecialCollectionCache struct {
	Intent *intents.Intent
	Demux  *Demultiplexer
	bytes.Buffer
}

// Open is part of the both interfaces, and it does nothing
func (cache *SpecialCollectionCache) Open() error {
	return nil
}

// Close is part of the both interfaces, and it does nothing
func (cache *SpecialCollectionCache) Close() error {
	cache.Intent.Size = int64(cache.Buffer.Len())
	return nil
}

// MutedCollection implements both DemuxOut as well as intents.file. It serves as a way to
// let the demutiplexer ignore certain embedded streams
type MutedCollection struct {
	Intent *intents.Intent
	Demux  *Demultiplexer
}

// Read is part of the intents.file interface, and does nothing
func (*MutedCollection) Read([]byte) (int, error) {
	// Read is part of the intents.file interface, and does nothing
	return 0, io.EOF
}

// Write is part of the intents.file interface, and does nothing
func (*MutedCollection) Write(b []byte) (int, error) {
	return len(b), nil
}

// Close is part of the intents.file interface, and does nothing
func (*MutedCollection) Close() error {
	return nil
}

// Open is part of the intents.file interface, and does nothing
func (*MutedCollection) Open() error {
	return nil
}

//===== Archive Manager Prioritizer =====

// NewPrioritizer careates a new Prioritizer and hooks up its Namespace channels to the ones in demux
func (demux *Demultiplexer) NewPrioritizer(mgr *intents.Manager) *Prioritizer {
	return &Prioritizer{
		NamespaceChan:      demux.NamespaceChan,
		NamespaceErrorChan: demux.NamespaceErrorChan,
		mgr:                mgr,
	}
}

// Prioritizer is a completely reactive prioritizer
// Intents are handed out as they arrive in the archive
type Prioritizer struct {
	NamespaceChan      <-chan string
	NamespaceErrorChan chan<- error
	mgr                *intents.Manager
}

// Get waits for a new namespace from the NamespaceChan, and returns a Intent found for it
func (prioritizer *Prioritizer) Get() *intents.Intent {
	namespace, ok := <-prioritizer.NamespaceChan
	if !ok {
		return nil
	}
	intent := prioritizer.mgr.IntentForNamespace(namespace)
	if intent == nil {
		prioritizer.NamespaceErrorChan <- fmt.Errorf("no intent for namespace %v", namespace)
	} else {
		if intent.BSONPath != "" {
			intent.BSONFile.Open()
		}
		prioritizer.NamespaceErrorChan <- nil
	}
	return intent
}

// Finish is part of the IntentPrioritizer interface, and does nothing
func (prioritizer *Prioritizer) Finish(*intents.Intent) {
	// no-op
	return
}
