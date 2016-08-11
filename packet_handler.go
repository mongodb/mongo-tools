package mongotape

import (
	"io"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
)

// PacketHandler wraps pcap.Handle to maintain other useful information.
type PacketHandler struct {
	Verbose    bool
	pcap       *pcap.Handle
	numDropped int64
	stop       chan struct{}
}

// NewPacketHandler initializes a new PacketHandler
func NewPacketHandler(pcapHandle *pcap.Handle) *PacketHandler {
	return &PacketHandler{
		pcap: pcapHandle,
		stop: make(chan struct{}),
	}
}

// StreamHandler is an io.Closer for a tcpassembly.StreamFactory
type StreamHandler interface {
	tcpassembly.StreamFactory
	io.Closer
}

// SetFirstSeener is an interface for anything which maintains the appropriate
// 'first seen' information.
type SetFirstSeener interface {
	SetFirstSeen(t time.Time)
}

// Close stops the packetHandler
func (p *PacketHandler) Close() {
	p.stop <- struct{}{}
}

func bookkeep(pktCount uint, pkt gopacket.Packet, assembler *Assembler) {
	if pkt != nil {
		userInfoLogger.Logf(DebugLow, "processed packet %7.v with timestamp %v", pktCount, pkt.Metadata().Timestamp.Format(time.RFC3339))
		assembler.FlushOlderThan(pkt.Metadata().CaptureInfo.Timestamp.Add(time.Minute * -5))
	}
}

// Handle reads the pcap file into assembled packets for the streamHandler
func (p *PacketHandler) Handle(streamHandler StreamHandler, numToHandle int) error {
	count := int64(0)
	start := time.Now()
	if p.Verbose && numToHandle > 0 {
		userInfoLogger.Logf(Always, "Processing", numToHandle, "packets")
	}
	source := gopacket.NewPacketSource(p.pcap, p.pcap.LinkType())
	streamPool := NewStreamPool(streamHandler)
	assembler := NewAssembler(streamPool)
	defer func() {
		if userInfoLogger.isInVerbosity(DebugLow) {
			userInfoLogger.Log(DebugLow, "flushing assembler.")
			userInfoLogger.Logf(DebugLow, "num flushed/closed: %v", assembler.FlushAll())
			userInfoLogger.Log(DebugLow, "closing stream handler.")
		} else {
			assembler.FlushAll()
		}
		streamHandler.Close()
	}()
	defer func() {
		if userInfoLogger.isInVerbosity(DebugLow) {
			userInfoLogger.Logf(DebugLow, "Dropped %v packets out of %v", p.numDropped, count)
			runTime := float64(time.Now().Sub(start)) / float64(time.Second)
			userInfoLogger.Logf(DebugLow, "Processed %v packets per second", float64(count-p.numDropped)/runTime)
		}
	}()
	ticker := time.Tick(time.Second * 1)
	var pkt gopacket.Packet
	var pktCount uint
	for {
		select {
		case pkt = <-source.Packets():
			pktCount++
			if pkt == nil { // end of pcap file
				userInfoLogger.Log(DebugLow, "Reached end of stream")
				return nil
			}
			if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
				userInfoLogger.Log(DebugHigh, "Assembling TCP layer")
				assembler.AssembleWithTimestamp(
					pkt.TransportLayer().TransportFlow(),
					tcpLayer.(*layers.TCP),
					pkt.Metadata().Timestamp) // TODO: use time.Now() here when running in realtime mode
			}
			if count == 0 {
				if firstSeener, ok := streamHandler.(SetFirstSeener); ok {
					firstSeener.SetFirstSeen(pkt.Metadata().Timestamp)
				}
			}
			count++
			if numToHandle > 0 && count >= int64(numToHandle) {
				userInfoLogger.Log(DebugLow, "Count exceeds requested packets, returning.")
				break
			}
			select {
			case <-ticker:
				bookkeep(pktCount, pkt, assembler)
			default:
			}
		case <-ticker:
			bookkeep(pktCount, pkt, assembler)
		case <-p.stop:
			return nil
		}
	}
}
