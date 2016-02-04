package mongoplay

import (
	"github.com/10gen/llmgo/bson"
	"github.com/google/gopacket/pcap"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"

	"fmt"
	"os"
)

type RecordCommand struct {
	GlobalOpts       *Options `no-flag:"true"`
	PcapFile         string   `short:"f" description:"path to the pcap file to be read"`
	PlaybackFile     string   `short:"p" description:"path to playback file to record to" long:"playback-file" required:"yes"`
	NetworkInterface string   `short:"i" description:"network interface to listen on"`
	PacketBufSize    int      `short:"b" description:"Size of heap used to merge separate streams together" default:"1000"`
}

func (record *RecordCommand) Execute(args []string) error {
	// we want to default verbosity to 1 (info), so increment the default setting of 0
	record.GlobalOpts.Verbose = append(record.GlobalOpts.Verbose, true)
	log.SetVerbosity(&options.Verbosity{record.GlobalOpts.Verbose, false})

	pcap, err := pcap.OpenOffline(record.PcapFile)
	if err != nil {
		return fmt.Errorf("error opening pcap file: %v", err)
	}
	if record.PacketBufSize < 1 {
		return fmt.Errorf("invalid PacketBufSize")
	}
	output, err := os.Create(record.PlaybackFile)
	h := NewPacketHandler(pcap)
	m := NewMongoOpStream(record.PacketBufSize)

	ch := make(chan error)
	go func() {
		defer close(ch)
		for op := range m.Ops {
			bsonBytes, err := bson.Marshal(op)
			if err != nil {
				ch <- fmt.Errorf("error marshaling message: %v", err)
				return
			}
			_, err = output.Write(bsonBytes)
			if err != nil {
				ch <- fmt.Errorf("error writing message: %v", err)
				return
			}
		}
		ch <- nil
	}()

	if err := h.Handle(m, -1); err != nil {
		fmt.Errorf("record: error handling packet stream:", err)
	}
	return <-ch
}

func (record *RecordCommand) ValidateParams(args []string) error {
	switch {
	case len(args) > 0:
		return fmt.Errorf("unknown argument: %s", args[0])
	case record.PcapFile != "" && record.NetworkInterface != "":
		return fmt.Errorf("must only specify an interface or a pcap file")
	}
	return nil
}
