package mongotape

import (
	"io"
	"log"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
)

type PacketHandler struct {
	Verbose    bool
	pcap       *pcap.Handle
	numDropped int64
	stop       chan struct{}
}

func NewPacketHandler(pcapHandle *pcap.Handle) *PacketHandler {
	return &PacketHandler{
		pcap: pcapHandle,
		stop: make(chan struct{}),
	}
}

type StreamHandler interface {
	tcpassembly.StreamFactory
	io.Closer
}

type SetFirstSeener interface {
	SetFirstSeen(t time.Time)
}

func (p *PacketHandler) Close() {
	p.stop <- struct{}{}
}

func (p *PacketHandler) Handle(streamHandler StreamHandler, numToHandle int) error {
	count := int64(0)
	start := time.Now()
	if p.Verbose && numToHandle > 0 {
		log.Println("Processing", numToHandle, "packets")
	}
	source := gopacket.NewPacketSource(p.pcap, p.pcap.LinkType())
	streamPool := tcpassembly.NewStreamPool(streamHandler)
	assembler := tcpassembly.NewAssembler(streamPool)
	defer func() {
		if p.Verbose {
			log.Println("flushing assembler.")
			log.Println("num flushed/closed:", assembler.FlushAll())
			log.Println("closing stream handler.")
		} else {
			assembler.FlushAll()
		}
		streamHandler.Close()
	}()
	defer func() {
		if p.Verbose {
			log.Println("Dropped", p.numDropped, "packets out of", count)
			runTime := float64(time.Now().Sub(start)) / float64(time.Second)
			log.Println("Processed", float64(count-p.numDropped)/runTime, "packets per second")
		}
	}()
	ticker := time.Tick(time.Second * 1)
	for {
		select {
		case pkt := <-source.Packets():
			if pkt == nil { // end of pcap file
				if p.Verbose {
					log.Println("end of stream")
				}
				return nil
			}
			if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
				assembler.AssembleWithTimestamp(
					pkt.TransportLayer().TransportFlow(),
					tcpLayer.(*layers.TCP),
					pkt.Metadata().Timestamp)
			}
			if count == 0 {
				if firstSeener, ok := streamHandler.(SetFirstSeener); ok {
					firstSeener.SetFirstSeen(pkt.Metadata().Timestamp)
				}
			}
			count++
			if numToHandle > 0 && count >= int64(numToHandle) {
				if p.Verbose {
					log.Println("Count exceeds requested packets, returning.")
				}
				break
			}
		case <-ticker:
			if p.Verbose {
				log.Println("flushing old streams")
			}
			assembler.FlushOlderThan(time.Now().Add(time.Second * -5))
		case <-p.stop:
			assembler.FlushOlderThan(time.Now().Add(time.Minute * -2))
			return nil
		}
	}
	return nil
}
