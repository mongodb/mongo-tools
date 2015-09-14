package mongoproto

import (
	"errors"
	"io"
)

var (
	ErrInvalidSize = errors.New("mongoproto: got invalid document size")
)

const (
	maximumDocumentSize = 49 * 1024 * 1024 // there is a 48MB max message size
)

// CopyMessage copies reads & writes an entire message.
func CopyMessage(w io.Writer, r io.Reader) error {
	h, err := ReadHeader(r)
	if err != nil {
		return err
	}
	if err := h.WriteTo(w); err != nil {
		return err
	}
	_, err = io.CopyN(w, r, int64(h.MessageLength-MsgHeaderLen))
	return err
}

// ReadDocument read an entire BSON document. This document can be used with
// bson.Unmarshal.
func ReadDocument(r io.Reader) ([]byte, error) {
	var sizeRaw [4]byte
	if _, err := io.ReadFull(r, sizeRaw[:]); err != nil {
		return nil, err
	}
	size := getInt32(sizeRaw[:], 0)
	if size < 5 {
		return nil, ErrInvalidSize
	}
	if size > maximumDocumentSize {
		return nil, ErrInvalidSize
	}
	doc := make([]byte, size)
	if size == 0 {
		return doc, nil
	}
	if size < 4 {
		return doc, nil
	}
	setInt32(doc, 0, size)

	if _, err := io.ReadFull(r, doc[4:]); err != nil {
		return doc, err
	}
	return doc, nil
}

// readCStringFromReader reads a null turminated string from an io.Reader.
func readCStringFromReader(r io.Reader) ([]byte, error) {
	var b []byte
	var n [1]byte
	for {
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return nil, err
		}
		if n[0] == 0 {
			return b, nil
		}
		b = append(b, n[0])
	}
}

func readCString(b []byte) string {
	for i := 0; i < len(b); i++ {
		if b[i] == 0 {
			return string(b[:i])
		}
	}
	return ""
}

// all data in the MongoDB wire protocol is little-endian.
// all the read/write functions below are little-endian.
func getInt32(b []byte, pos int) int32 {
	return (int32(b[pos+0])) |
		(int32(b[pos+1]) << 8) |
		(int32(b[pos+2]) << 16) |
		(int32(b[pos+3]) << 24)
}

func setInt32(b []byte, pos int, i int32) {
	b[pos] = byte(i)
	b[pos+1] = byte(i >> 8)
	b[pos+2] = byte(i >> 16)
	b[pos+3] = byte(i >> 24)
}

func getInt64(b []byte, pos int) int64 {
	return (int64(b[pos+0])) |
		(int64(b[pos+1]) << 8) |
		(int64(b[pos+2]) << 16) |
		(int64(b[pos+3]) << 24) |
		(int64(b[pos+4]) << 32) |
		(int64(b[pos+5]) << 40) |
		(int64(b[pos+6]) << 48) |
		(int64(b[pos+7]) << 56)
}
