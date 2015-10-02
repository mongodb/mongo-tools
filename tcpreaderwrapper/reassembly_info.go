// Package tcpreaderwrapper wraps a gopacket tcpassembly.tcpreader.ReaderStream
package tcpreaderwrapper

import (
	"time"

	"github.com/google/gopacket/tcpassembly"
)

// ReassemblyInfo represents the metadata about a tcpassembly.Reassembly
// (it does not store the actual bytes)
// It duplicates the fields from Reassembly but stores number of bytes instead
// of actual bytes
type ReassemblyInfo struct {
	// Bytes is number of bytes in the next reassembled part of the associated
	// Stream.
	NumBytes int
	// Skip is set to non-zero if bytes were skipped between this and the
	// last Reassembly.  If this is the first packet in a connection and we
	// didn't see the start, we have no idea how many bytes we skipped, so
	// we set it to -1.  Otherwise, it's set to the number of bytes skipped.
	Skip int
	// Start is set if this set of bytes has a TCP SYN accompanying it.
	Start bool
	// End is set if this set of bytes has a TCP FIN or RST accompanying it.
	End bool
	// Seen is the timestamp this set of bytes was pulled off the wire.
	Seen time.Time
}

// newReassemblyInfo creates a ReassemblyInfo from an existing Reassembly
func newReassemblyInfo(r tcpassembly.Reassembly) ReassemblyInfo {
	return ReassemblyInfo{
		NumBytes: len(r.Bytes),
		Skip:     r.Skip,
		Start:    r.Start,
		End:      r.End,
		Seen:     r.Seen,
	}
}
