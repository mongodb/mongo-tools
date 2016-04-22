package mongotape

import (
	"compress/gzip"
	"github.com/10gen/llmgo/bson"
	"github.com/google/gopacket/pcap"
	"io"
	"os/signal"

	"fmt"
	"os"
	"syscall"
)

type RecordCommand struct {
	GlobalOpts       *Options `no-flag:"true"`
	PcapFile         string   `short:"f" description:"path to the pcap file to be read"`
	Expression       string   `short:"e" long:"expr" description:"BPF filter expression to apply to packets for recording"`
	Gzip             bool     `long:"gzip" description:"compress output file with Gzip"`
	PlaybackFile     string   `short:"p" description:"path to playback file to record to" long:"playback-file" required:"yes"`
	NetworkInterface string   `short:"i" description:"network interface to listen on"`
	PacketBufSize    int      `short:"b" description:"Size of heap used to merge separate streams together" default:"1000"`
}

type opStreamSettings struct {
	networkInterface string
	pcapFile         string
	packetBufSize    int
	expression       string
}

type packetHandlerContext struct {
	packetHandler *PacketHandler
	mongoOpStream *MongoOpStream
	pcapHandle    *pcap.Handle
}

func getOpstream(cfg opStreamSettings) (*packetHandlerContext, error) {
	if cfg.packetBufSize < 1 {
		return nil, fmt.Errorf("invalid packet buffer size")
	}

	var pcapHandle *pcap.Handle
	var err error
	if len(cfg.pcapFile) > 0 {
		pcapHandle, err = pcap.OpenOffline(cfg.pcapFile)
		if err != nil {
			return nil, fmt.Errorf("error opening pcap file: %v", err)
		}
	} else if len(cfg.networkInterface) > 0 {
		pcapHandle, err = pcap.OpenLive(cfg.networkInterface, 32*1024*1024, false, pcap.BlockForever)
		if err != nil {
			return nil, fmt.Errorf("error listening to network interface: %v", err)
		}
	} else {
		return nil, fmt.Errorf("must specify either a pcap file or network interface to record from")
	}

	if len(cfg.expression) > 0 {
		err = pcapHandle.SetBPFFilter(cfg.expression)
		if err != nil {
			return nil, fmt.Errorf("error setting packet filter expression: %v", err)
		}
	}

	h := NewPacketHandler(pcapHandle)
	h.Verbose = userInfoLogger.isInVerbosity(DebugLow)

	m := NewMongoOpStream(cfg.packetBufSize)
	return &packetHandlerContext{h, m, pcapHandle}, nil
}

type PlaybackWriter struct {
	io.WriteCloser
	fname string
}

func (record *RecordCommand) NewPlaybackWriter() (*PlaybackWriter, error) {
	pbWriter := &PlaybackWriter{
		fname: record.PlaybackFile,
	}
	file, err := os.Create(pbWriter.fname)
	if err != nil {
		return nil, fmt.Errorf("error opening playback file to write to: %v", err)
	}
	if record.Gzip {
		pbWriter.WriteCloser = gzip.NewWriter(file)
	} else {
		pbWriter.WriteCloser = file
	}
	return pbWriter, nil
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
	toolDebugLogger.Logf(DebugLow, "Opening playback file %v", record.PlaybackFile)

	ctx, err := getOpstream(opStreamSettings{
		networkInterface: record.NetworkInterface,
		pcapFile:         record.PcapFile,
		packetBufSize:    record.PacketBufSize,
		expression:       record.Expression,
	})
	if err != nil {
		return err
	}

	// When a signal is received to kill the process, stop the packet handler so we
	// gracefully flush all ops being processed before exiting.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		// Block until a signal is received.
		s := <-sigChan
		toolDebugLogger.Logf(Info, "Got signal %v, closing PCAP handle", s)
		ctx.packetHandler.Close()
	}()
	playbackWriter, err := record.NewPlaybackWriter()
	if err != nil {
		return err
	}

	ch := make(chan error)
	go func() {
		defer close(ch)
		for op := range ctx.mongoOpStream.Ops {
			bsonBytes, err := bson.Marshal(op)
			if err != nil {
				ch <- fmt.Errorf("error marshaling message: %v", err)
				return
			}
			_, err = playbackWriter.Write(bsonBytes)
			if err != nil {
				ch <- fmt.Errorf("error writing message: %v", err)
				return
			}
		}
		ch <- nil
	}()

	if err := ctx.packetHandler.Handle(ctx.mongoOpStream, -1); err != nil {
		fmt.Errorf("record: error handling packet stream:", err)
	}

	stats, err := ctx.pcapHandle.Stats()
	if err != nil {
		toolDebugLogger.Logf(Always, "Warning: got err %v getting pcap handle stats", err)
	} else {
		toolDebugLogger.Logf(Info, "PCAP stats: %#v", stats)
	}

	return <-ch
}
