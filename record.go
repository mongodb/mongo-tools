package mongoplay

import (
	"fmt"
	"github.com/10gen/llmgo/bson"
	"github.com/google/gopacket/pcap"
	"os"
)

type RecordCommand struct {
	GlobalOpts   *Options `no-flag:"true"`
	PlaybackFile struct {
		PlaybackFile string
	} `required:"yes" positional-args:"yes" description:"path to the playback file to write to"`
	PcapFile         string `short:"f" description:"path to the pcap file to be read"`
	NetworkInterface string `short:"i" description:"network interface to listen on"`
	PacketBufSize    int
}

func (record *RecordCommand) Execute(args []string) error {
	pcap, err := pcap.OpenOffline(record.PcapFile)
	if err != nil {
		return fmt.Errorf("error opening pcap file: %v", err)
	}
	output, err := os.Create(record.PlaybackFile.PlaybackFile)
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
