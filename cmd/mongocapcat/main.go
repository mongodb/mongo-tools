package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/10gen/mongoplay"
	"github.com/10gen/mongoplay/mongoproto"
	"github.com/google/gopacket/pcap"
)

var (
	pcapFile      = flag.String("f", "-", "pcap file (or '-' for stdin)")
	packetBufSize = flag.Int("size", 1000, "size of packet buffer used for ordering within streams")
	verbose       = flag.Bool("v", false, "verbose output (to stderr)")
)

func main() {
	flag.Parse()

	pcap, err := pcap.OpenOffline(*pcapFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening pcap file:", err)
		os.Exit(1)
	}
	h := mongocaputils.NewPacketHandler(pcap)
	m := mongocaputils.NewMongoOpStream(*packetBufSize)

	ch := make(chan struct{})
	go func() {
		defer close(ch)
		for op := range m.Ops {
			if _, ok := op.Op.(*mongoproto.OpUnknown); !ok {
				fmt.Printf("%f %v\n", float64(op.Seen.Sub(m.FirstSeen))/10e8, op)
			}
		}
	}()

	if err := h.Handle(m, -1); err != nil {
		fmt.Fprintln(os.Stderr, "mongocapcat: error handling packet stream:", err)
	}
	<-ch
}
