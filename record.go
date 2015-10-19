package mongoplay

import (
	"fmt"
	"log"
	"os"

	"github.com/10gen/llmgo/bson"
	"github.com/google/gopacket/pcap"
	"github.com/jessevdk/go-flags"
)

type RecordOptions struct {
	PlaybackFile struct {
		PlaybackFile string
	} `required:"yes" positional-args:"yes"`
	PcapFile         string `short:"f" required:"yes"`
	NetworkInterface string
	PacketBufSize    int
	Verbose          bool
}

type RecordConf struct {
	RecordOptions
	Logger *log.Logger
}

func (record *RecordConf) ParseRecordFlags(args []string) error {
	_, err := flags.ParseArgs(record, args)
	return err
	// TODO figure out what to do here when there are extra args
}

func (record *RecordConf) Record() error {
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
