package main

import (
	"flag"
	"fmt"
	"gopkg.in/mgo.v2/bson"
	"os"

	"github.com/gabrielrussell/mongocaputils"
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
			bsonBytes, err := bson.Marshal(op)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error marshaling message:", err)
				os.Exit(1)
			}
			_, err = os.Stdout.Write(bsonBytes)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error writing message:", err)
				os.Exit(1)
			}
		}
	}()

	if err := h.Handle(m, -1); err != nil {
		fmt.Fprintln(os.Stderr, "mongorecord: error handling packet stream:", err)
	}
	<-ch
}
