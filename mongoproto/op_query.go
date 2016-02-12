package mongoproto

import (
	"fmt"
	"io"
	"reflect"
	"time"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/mongodb/mongo-tools/common/json"
	"strings"
)

// OpQuery is used to query the database for documents in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-query
type QueryOp struct {
	Header MsgHeader
	mgo.QueryOp
}

func (op *QueryOp) Meta() OpMetadata {
	opType, commandType := extractOpType(op.QueryOp.Query)
	if !strings.HasSuffix(op.Collection, "$cmd") {
		return OpMetadata{"query", op.Collection, ""}
	}

	return OpMetadata{opType, op.Collection, commandType}
}

// extractOpType checks a write command's "query" and determines if it's actually
// an insert, update, delete, or command.
func extractOpType(x interface{}) (string, string) {
	var asMap bson.M
	var commandName string
	switch v := x.(type) {
	case bson.D:
		if len(v) > 0 {
			commandName = v[0].Name
		}
		asMap = v.Map()
	case *bson.M: // document
		asMap = *v
	case bson.M: // document
		asMap = v
	case map[string]interface{}:
		asMap = bson.M(v)
	case (*bson.D):
		if de := []bson.DocElem(*v); len(de) > 0 {
			commandName = de[0].Name
		}
		asMap = v.Map()
	}

	for _, v := range []string{"insert", "update", "delete"} {
		if _, ok := asMap[v]; ok {
			return v, ""
		}
	}
	return "command", commandName
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

	op.Query = &bson.Raw{}
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

func (op *QueryOp) Execute(session *mgo.Session) (*OpResult, error) {
	session.SetSocketTimeout(0)
	before := time.Now()
	data, reply, err := mgo.ExecOpWithReply(session, &op.QueryOp)
	after := time.Now()
	if err != nil {
		fmt.Printf("query error: %v\n", err)
	}

	result := &OpResult{reply, make([]bson.Raw, 0, len(data)), after.Sub(before)}
	for _, d := range data {
		dataDoc := bson.Raw{}
		err = bson.Unmarshal(d, &dataDoc)
		if err != nil {
			return nil, err
		}
		result.Docs = append(result.Docs, dataDoc)
	}
	return result, nil
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
