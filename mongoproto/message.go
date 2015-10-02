package mongoproto

import (
	"fmt"
	"io"
)

const MsgHeaderLen = 16 // mongo MsgHeader length in bytes

// MsgHeader is the mongo MessageHeader
type MsgHeader struct {
	// MessageLength is the total message size, including this header
	MessageLength int32
	// RequestID is the identifier for this miessage
	RequestID int32
	// ResponseTo is the RequestID of the message being responded to. used in DB responses
	ResponseTo int32
	// OpCode is the request type, see consts above.
	OpCode OpCode
}

func ReadHeader(r io.Reader) (*MsgHeader, error) {
	var d [MsgHeaderLen]byte
	b := d[:]
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	h := MsgHeader{}
	h.fromWire(b)
	return &h, nil
}

// toWire converts the MsgHeader to the wire protocol
func (m MsgHeader) toWire() []byte {
	var d [MsgHeaderLen]byte
	b := d[:]
	setInt32(b, 0, m.MessageLength)
	setInt32(b, 4, m.RequestID)
	setInt32(b, 8, m.ResponseTo)
	setInt32(b, 12, int32(m.OpCode))
	return b
}

// fromWire reads the wirebytes into this object
func (m *MsgHeader) fromWire(b []byte) {
	m.MessageLength = getInt32(b, 0)
	m.RequestID = getInt32(b, 4)
	m.ResponseTo = getInt32(b, 8)
	m.OpCode = OpCode(getInt32(b, 12))
}

func (m *MsgHeader) WriteTo(w io.Writer) error {
	b := m.toWire()
	n, err := w.Write(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return fmt.Errorf("mongoproto: attempted to write %d but wrote %d", len(b), n)
	}
	return nil
}

// String returns a string representation of the message header. Useful for debugging.
func (m *MsgHeader) String() string {
	return fmt.Sprintf(
		"opCode:%s (%d) msgLen:%d reqID:%d respID:%d",
		m.OpCode,
		m.OpCode,
		m.MessageLength,
		m.RequestID,
		m.ResponseTo,
	)
}
