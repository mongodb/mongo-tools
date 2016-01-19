package mongoproto

import (
	"fmt"
	"io"
	"reflect"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/mongodb/mongo-tools/common/json"
)

// OpQuery is used to query the database for documents in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-query
type QueryOp struct {
	Header MsgHeader
	mgo.QueryOp
}

func (op *QueryOp) String() string {
	queryAsJSON, err := ConvertBSONValueToJSON(op.Query)
	if err != nil {
		return fmt.Sprintf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}
	asJSON, err := json.Marshal(queryAsJSON)
	if err != nil {
		return fmt.Sprintf("json marshal err: %#v - %v", op, err)
	}
	return fmt.Sprintf("OpQuery %v %v", op.Collection, string(asJSON))
}

func (op *QueryOp) OpCode() OpCode {
	return OpCodeQuery
}

func (op *QueryOp) FromReader(r io.Reader) error {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return err
	}
	op.Flags = mgo.QueryOpFlags(getInt32(b[:], 0))
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Collection = string(name)

	if _, err := io.ReadFull(r, b[:]); err != nil {
		return err
	}

	op.Skip = getInt32(b[:], 0)
	op.Limit = getInt32(b[:], 4)

	queryAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}

	op.Query = &bson.D{}
	err = bson.Unmarshal(queryAsSlice, op.Query)
	if err != nil {
		return err
	}
	currentRead := len(queryAsSlice) + len(op.Collection) + 1 + 12 + MsgHeaderLen
	if int(op.Header.MessageLength) > currentRead {
		selectorAsSlice, err := ReadDocument(r)
		if err != nil {
			return err
		}
		op.Selector = &bson.D{}
		err = bson.Unmarshal(selectorAsSlice, op.Selector)
		if err != nil {
			return err
		}
	}
	return nil
}

func (op *QueryOp) Execute(session *mgo.Session) (*mgo.ReplyOp, error) {
	data, reply, err := mgo.ExecOpWithReply(session, &op.QueryOp)
	if err != nil {
		fmt.Printf("query error: %v\n", err)
	}
	for _, d := range data {
		dataDoc := bson.D{}
		err = bson.Unmarshal(d, &dataDoc)
		if err != nil {
			return nil, err
		}
	}
	//fmt.Printf("reply: %#v\n", reply)

	return reply, nil
}

func (queryOp1 *QueryOp) Equals(otherOp Op) bool {
	queryOp2, ok := otherOp.(*QueryOp)
	if !ok {
		return false
	}
	switch {
	case queryOp1.Collection != queryOp2.Collection:
		return false
	case queryOp1.Skip != queryOp2.Skip:
		return false
	case queryOp1.Limit != queryOp2.Limit:
		return false
	case queryOp1.Flags != queryOp2.Flags:
		return false
	case !reflect.DeepEqual(queryOp1.Query, queryOp2.Query):
		return false
	default:
		return true
	}
}
