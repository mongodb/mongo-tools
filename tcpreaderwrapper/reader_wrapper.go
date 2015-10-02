// package tcpreaderwrapper wraps a gopacket tcpassembly.tcpreader.ReaderStream
// and holds recent resassemblies to fetch timing information
package tcpreaderwrapper

import (
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

type ReaderStreamWrapper struct {
	tcpreader.ReaderStream
	Reassemblies []ReassemblyInfo
}

// NewReaderStream returns a new ReaderStreamWrapper object.
func NewReaderStreamWrapper() ReaderStreamWrapper {
	r := ReaderStreamWrapper{
		ReaderStream: tcpreader.NewReaderStream(),
		Reassemblies: make([]ReassemblyInfo, 0),
	}
	//	r.ReaderStream.ReaderStreamOptions.LossErrors = true
	return r
}

// Reassembled implements tcpassembly.Stream's Reassembled function.
func (r *ReaderStreamWrapper) Reassembled(reassembly []tcpassembly.Reassembly) {
	// keep track of sizes and times to reconstruct
	for _, re := range reassembly {
		r.Reassemblies = append(r.Reassemblies, newReassemblyInfo(re))
	}
	r.ReaderStream.Reassembled(reassembly)
}
