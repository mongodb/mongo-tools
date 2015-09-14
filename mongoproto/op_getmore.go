package mongoproto

import (
	"gopkg.in/mgo.v2"
	"io"
)

// OpGetMore is used to query the database for documents in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-get-more
type OpGetMore struct {
	Header             MsgHeader
	FullCollectionName string // "dbname.collectionname"
	NumberToReturn     int32  // number of documents to return
	CursorID           int64  // cursorID from the OpReply
}

func (op *OpGetMore) OpCode() OpCode {
	return OpCodeGetMore
}

func (op *OpGetMore) FromReader(r io.Reader) error {
	var b [12]byte
	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return err
	}
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.FullCollectionName = string(name)
	if _, err := io.ReadFull(r, b[:12]); err != nil {
		return err
	}
	op.NumberToReturn = getInt32(b[:], 0)
	op.CursorID = getInt64(b[:], 4)
	return nil
}

func (op *OpGetMore) fromWire(b []byte) {
	b = b[4:] // skip ZERO
	op.FullCollectionName = readCString(b)
	b = b[len(op.FullCollectionName)+1:]
	op.NumberToReturn = getInt32(b, 0)
	op.CursorID = getInt64(b, 4)
}

func (op *OpGetMore) toWire() []byte {
	return nil
}

func (op *OpGetMore) Execute(session *mgo.Session) error {
	return nil
}
