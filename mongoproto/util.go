package mongoproto

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

// all data in the MongoDB wire protocol is little-endian.
// all the read/write functions below are little-endian.
func getInt32(b []byte, pos int) int32 {
	return (int32(b[pos+0])) |
		(int32(b[pos+1]) << 8) |
		(int32(b[pos+2]) << 16) |
		(int32(b[pos+3]) << 24)
}

func SetInt32(b []byte, pos int, i int32) {
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
