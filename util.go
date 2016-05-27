package mongotape

import (
	"encoding/base64"
	"errors"
	"fmt"
	mgo "github.com/10gen/llmgo"
	bson "github.com/10gen/llmgo/bson"
	"github.com/mongodb/mongo-tools/common/json"
	"io"
	"reflect"
	"time"
)

var (
	ErrInvalidSize = errors.New("got invalid document size")
)

const (
	maximumDocumentSize = 49 * 1024 * 1024 // there is a 48MB max message size
)

// GetCursorId examines a set of docs to look for a command reply that contains a cursor ID, and
// returns it if found. Returns 0 if no cursor ID was found. May return an error if the given
// documents contain invalid or corrupt bson.
func GetCursorId(op *mgo.ReplyOp, docs []bson.Raw) (int64, error) {
	if op == nil {
		return 0, nil
	}
	if op.CursorId != 0 {
		return op.CursorId, nil
	}
	if len(docs) != 1 {
		return 0, nil
	}
	firstDoc := bson.M{}
	err := docs[0].Unmarshal(&firstDoc)
	if err != nil {
		// can happen if there's corrupt bson in the doc.
		return 0, fmt.Errorf("failed to unmarshal raw into bson.D: %v", err)
	}
	cursorInfo := firstDoc["cursor"]
	if cursorInfo == nil {
		return 0, nil
	}
	var cursorInfoMap bson.M
	var ok bool
	if cursorInfoMap, ok = cursorInfo.(bson.M); !ok {
		return 0, nil
	}
	if cursorId, ok := cursorInfoMap["id"]; ok {
		switch v := cursorId.(type) {
		case int64:
			return v, nil
		default:
			return 0, nil
		}
	}

	return 0, nil
}

// Abbreviate returns a reduced copy of the given string if it's longer than maxLen by
// showing only a prefix and suffix of size
// windowLen with an ellipsis in the middle.
func Abbreviate(data string, maxLen int) string {
	if len(data) <= maxLen {
		return data
	}
	windowLen := (maxLen - 3) / 2
	return data[0:windowLen] + "..." + data[len(data)-windowLen:]
}

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
	SetInt32(doc, 0, size)

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

//retrieves a 32 bit into from the given byte array whose first byte is in position pos
//Taken from gopkg.in/mgo.v2/socket.go
func getInt32(b []byte, pos int) int32 {
	return (int32(b[pos+0])) |
		(int32(b[pos+1]) << 8) |
		(int32(b[pos+2]) << 16) |
		(int32(b[pos+3]) << 24)
}

//sets 32 bit int into the given byte array at position post
//Taken from gopkg.in/mgo.v2/socket.go
func SetInt32(b []byte, pos int, i int32) {
	b[pos] = byte(i)
	b[pos+1] = byte(i >> 8)
	b[pos+2] = byte(i >> 16)
	b[pos+3] = byte(i >> 24)
}

//retrieves a 64 bit into from the given byte array whose first byte is in position pos
//Taken from gopkg.in/mgo.v2/socket.go
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

func convertKeys(v bson.M) (bson.M, error) {
	for key, value := range v {
		jsonValue, err := ConvertBSONValueToJSON(value)
		if err != nil {
			return nil, err
		}
		v[key] = jsonValue
	}
	return v, nil
}

//sets 64 bit int into the given byte array at position post
//Taken from gopkg.in/mgo.v2/socket.go
func SetInt64(b []byte, pos int, i int64) {
	b[pos] = byte(i)
	b[pos+1] = byte(i >> 8)
	b[pos+2] = byte(i >> 16)
	b[pos+3] = byte(i >> 24)
	b[pos+4] = byte(i >> 32)
	b[pos+5] = byte(i >> 40)
	b[pos+6] = byte(i >> 48)
	b[pos+7] = byte(i >> 56)
}

// ConvertBSONValueToJSON walks through a document or an array and
// converts any BSON value to its corresponding extended JSON type.
// It returns the converted JSON document and any error encountered.
func ConvertBSONValueToJSON(x interface{}) (interface{}, error) {
	switch v := x.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil
	case *bson.M: // document
		doc, err := convertKeys(*v)
		if err != nil {
			return nil, err
		}
		return doc, err
	case bson.M: // document
		return convertKeys(v)
	case map[string]interface{}:
		return convertKeys(v)
	case []bson.Raw:
		out := make([]interface{}, len(v))
		for i, value := range v {
			out[i] = value
		}
		return ConvertBSONValueToJSON(out)
	case bson.Raw:
		// Unmarshal the raw into a bson.D, then process that.
		convertedFromRaw := bson.D{}
		err := v.Unmarshal(&convertedFromRaw)
		if err != nil {
			return nil, err
		}
		return ConvertBSONValueToJSON(convertedFromRaw)
	case (*bson.Raw):
		return ConvertBSONValueToJSON(*v)
	case (*bson.D):
		return ConvertBSONValueToJSON(*v)
	case bson.D:
		for i, value := range v {
			jsonValue, err := ConvertBSONValueToJSON(value.Value)
			if err != nil {
				return nil, err
			}
			v[i].Value = jsonValue
		}
		return v.Map(), nil
	case []bson.D:
		out := make([]interface{}, len(v))
		for i, value := range v {
			out[i] = value
		}
		return ConvertBSONValueToJSON(out)
	case []interface{}: // array
		for i, value := range v {
			jsonValue, err := ConvertBSONValueToJSON(value)
			if err != nil {
				return nil, err
			}
			v[i] = jsonValue
		}
		return v, nil
	case string:
		return v, nil // require no conversion

	case int:
		return json.NumberInt(v), nil

	case bson.ObjectId: // ObjectId
		return json.ObjectId(v.Hex()), nil

	case time.Time: // Date
		return json.Date(v.Unix()*1000 + int64(v.Nanosecond()/1e6)), nil

	case int64: // NumberLong
		return json.NumberLong(v), nil

	case int32: // NumberInt
		return json.NumberInt(v), nil

	case float64:
		return json.NumberFloat(v), nil

	case float32:
		return json.NumberFloat(float64(v)), nil

	case []byte: // BinData (with generic type)
		data := base64.StdEncoding.EncodeToString(v)
		return json.BinData{0x00, data}, nil

	case bson.Binary: // BinData
		data := base64.StdEncoding.EncodeToString(v.Data)
		return json.BinData{v.Kind, data}, nil

	case mgo.DBRef: // DBRef
		return map[string]interface{}{"$ref": v.Collection, "$id": v.Id}, nil

	//case bson.DBPointer: // DBPointer
	//return json.DBPointer{v.Namespace, v.Id}, nil

	case bson.RegEx: // RegExp
		return json.RegExp{v.Pattern, v.Options}, nil

	case bson.MongoTimestamp: // Timestamp
		timestamp := int64(v)
		return json.Timestamp{
			Seconds:   uint32(timestamp >> 32),
			Increment: uint32(timestamp),
		}, nil

	case bson.JavaScript: // JavaScript
		var scope interface{}
		var err error
		if v.Scope != nil {
			scope, err = ConvertBSONValueToJSON(v.Scope)
			if err != nil {
				return nil, err
			}
		}
		return json.JavaScript{v.Code, scope}, nil

	default:
		switch x {
		case bson.MinKey: // MinKey
			return json.MinKey{}, nil

		case bson.MaxKey: // MaxKey
			return json.MaxKey{}, nil

		case bson.Undefined: // undefined
			return json.Undefined{}, nil
		}
	}

	return nil, fmt.Errorf("conversion of BSON type '%v' not supported %v", reflect.TypeOf(x), x)
}

type PreciseTime struct {
	time.Time
}

type preciseTimeDecoder struct {
	Sec  int64 `bson:"sec"`
	Nsec int32 `bson:"nsec"`
}

const (
	// Time.Unix() returns the number of seconds from the unix epoch
	// but time's underlying struct stores 'sec' as the number of seconds
	// elapsed since January 1, year 1 00:00:00 UTC
	// This calculation allows for conversion between the internal representation
	// and the UTC representation
	unixToInternal int64 = (1969*365 + 1969/4 - 1969/100 + 1969/400) * 86400

	internalToUnix int64 = -unixToInternal
)

func (b *PreciseTime) GetBSON() (interface{}, error) {
	result := preciseTimeDecoder{
		Sec:  b.Unix() + unixToInternal,
		Nsec: int32(b.Nanosecond()),
	}
	return &result, nil

}

func (b *PreciseTime) SetBSON(raw bson.Raw) error {
	decoder := preciseTimeDecoder{}
	bsonErr := raw.Unmarshal(&decoder)
	if bsonErr != nil {
		return bsonErr
	}

	t := time.Unix(decoder.Sec+internalToUnix, int64(decoder.Nsec))
	b.Time = t.UTC()
	return nil
}
