package mongotape

import (
	"github.com/10gen/llmgo/bson"
	"github.com/google/gopacket/pcap"
	"os/signal"

	"fmt"
	"os"
	"syscall"
)

type RecordCommand struct {
	GlobalOpts       *Options `no-flag:"true"`
	PcapFile         string   `short:"f" description:"path to the pcap file to be read"`
	Expression       string   `short:"e" long:"expr" description:"BPF filter expression to apply to packets for recording"`
	PlaybackFile     string   `short:"p" description:"path to playback file to record to" long:"playback-file" required:"yes"`
	NetworkInterface string   `short:"i" description:"network interface to listen on"`
	PacketBufSize    int      `short:"b" description:"Size of heap used to merge separate streams together" default:"1000"`
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

func (record *RecordCommand) Execute(args []string) error {
	err := record.ValidateParams(args)
	if err != nil {
		return err
	}
	record.GlobalOpts.SetLogging()

	if record.PacketBufSize < 1 {
		return fmt.Errorf("invalid PacketBufSize")
	}

	var pcapHandle *pcap.Handle
	if len(record.PcapFile) > 0 {
		pcapHandle, err = pcap.OpenOffline(record.PcapFile)
		if err != nil {
			return fmt.Errorf("error opening pcap file: %v", err)
		}
	} else if len(record.NetworkInterface) > 0 {
		pcapHandle, err = pcap.OpenLive(record.NetworkInterface, 32*1024*1024, false, pcap.BlockForever)
		if err != nil {
			return fmt.Errorf("error listening to network interface: %v", err)
		}
	} else {
		return fmt.Errorf("must specify either a pcap file (-f) or network interface (-i) to record from")
	}

	if len(record.Expression) > 0 {
		err = pcapHandle.SetBPFFilter(record.Expression)
		if err != nil {
			return fmt.Errorf("error setting packet filter expression: %v", err)
		}
	}

	toolDebugLogger.Logf(DebugLow, "Opening playback file %v", record.PlaybackFile)
	output, err := os.Create(record.PlaybackFile)
	h := NewPacketHandler(pcapHandle)
	m := NewMongoOpStream(record.PacketBufSize)

	// When a signal is received to kill the process, stop the packet handler so we
	// gracefully flush all ops being processed before exiting.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		// Block until a signal is received.
		s := <-sigChan
		toolDebugLogger.Logf(Info, "Got signal %v, closing PCAP handle", s)
		h.Close()
	}()

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

	stats, err := pcapHandle.Stats()
	if err != nil {
		toolDebugLogger.Logf(Always, "Warning: got err %v getting pcap handle stats", err)
	} else {
		toolDebugLogger.Logf(Info, "PCAP stats: %#v", stats)
	}

	return <-ch
}
