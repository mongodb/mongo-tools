package bsontools

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// MarshalD marshals a bson.D to raw BSON, yielding the same result as
// bson.Marshal(). This avoids reflection, though, which significantly
// reduces CPU load. It also marshals to a preexisting buffer, which lets
// you minimize GC churn.
func MarshalD(buf []byte, d bson.D) (bson.Raw, error) {
	sizeAt := len(buf)

	buf = append(buf, 0, 0, 0, 0)

	var err error

	for _, el := range d {
		buf, err = marshalEl(buf, el.Key, el.Value)
		if err != nil {
			return nil, fmt.Errorf("marshaling %#q: %w", el.Key, err)
		}
	}

	buf = append(buf, 0)

	binary.LittleEndian.PutUint32(buf[sizeAt:], uint32(len(buf[sizeAt:])))

	return buf, nil
}

// MarshalA is like MarshalD but for a bson.A.
func MarshalA(buf []byte, a bson.A) (bson.RawArray, error) {
	sizeAt := len(buf)

	buf = append(buf, 0, 0, 0, 0)

	var err error

	for e, el := range a {
		key := strconv.Itoa(e)

		buf, err = marshalEl(buf, key, el)
		if err != nil {
			return nil, fmt.Errorf("marshaling element %d: %w", e, err)
		}
	}

	buf = append(buf, 0)

	binary.LittleEndian.PutUint32(buf[sizeAt:], uint32(len(buf[sizeAt:])))

	return buf, nil
}

func marshalEl(buf []byte, key string, val any) ([]byte, error) {
	switch typedVal := val.(type) {
	case float64:
		buf = bsoncore.AppendDoubleElement(buf, key, typedVal)
	case string:
		buf = bsoncore.AppendStringElement(buf, key, typedVal)
	case bson.D:
		buf = append(buf, byte(bson.TypeEmbeddedDocument))
		buf = append(buf, []byte(key)...)
		buf = append(buf, 0)

		var err error
		buf, err = MarshalD(buf, typedVal)
		if err != nil {
			return nil, fmt.Errorf("subdocument: %w", err)
		}
	case bson.A:
		buf = append(buf, byte(bson.TypeArray))
		buf = append(buf, []byte(key)...)
		buf = append(buf, 0)

		var err error
		buf, err = MarshalA(buf, typedVal)
		if err != nil {
			return nil, fmt.Errorf("subdocument: %w", err)
		}
	case bson.Binary:
		buf = bsoncore.AppendBinaryElement(buf, key, typedVal.Subtype, typedVal.Data)
	case bson.Undefined:
		buf = bsoncore.AppendUndefinedElement(buf, key)
	case bson.ObjectID:
		buf = bsoncore.AppendObjectIDElement(buf, key, typedVal)
	case bool:
		buf = bsoncore.AppendBooleanElement(buf, key, typedVal)

	// These are the same BSON type, but might as well accept both since
	// bsoncore has parsers for both:
	case time.Time:
		buf = bsoncore.AppendTimeElement(buf, key, typedVal)
	case bson.DateTime:
		buf = bsoncore.AppendDateTimeElement(buf, key, int64(typedVal))

	case bson.Null, nil:
		buf = bsoncore.AppendNullElement(buf, key)
	case bson.Regex:

		// To match the driver’s marshaling logic:
		opts := []rune(typedVal.Options)
		slices.Sort(opts)

		buf = bsoncore.AppendRegexElement(buf, key, typedVal.Pattern, string(opts))
	case bson.DBPointer:
		buf = bsoncore.AppendDBPointerElement(buf, key, typedVal.DB, typedVal.Pointer)
	case bson.JavaScript:
		buf = bsoncore.AppendJavaScriptElement(buf, key, string(typedVal))
	case bson.Symbol:
		buf = bsoncore.AppendSymbolElement(buf, key, string(typedVal))
	case bson.CodeWithScope:
		scope, err := bson.Marshal(typedVal.Scope)
		if err != nil {
			return nil, fmt.Errorf("code with scope, marshaling scope: %w", err)
		}

		buf = bsoncore.AppendCodeWithScopeElement(buf, key, string(typedVal.Code), scope)
	case int32:
		buf = bsoncore.AppendInt32Element(buf, key, typedVal)
	case bson.Timestamp:
		buf = bsoncore.AppendTimestampElement(buf, key, typedVal.T, typedVal.I)
	case int64:
		buf = bsoncore.AppendInt64Element(buf, key, typedVal)
	case bson.Decimal128:
		h, l := typedVal.GetBytes()
		buf = bsoncore.AppendDecimal128Element(buf, key, h, l)
	case bson.MaxKey:
		buf = bsoncore.AppendMaxKeyElement(buf, key)
	case bson.MinKey:
		buf = bsoncore.AppendMinKeyElement(buf, key)
	default:
		bType, raw, err := bson.MarshalValue(val)
		if err != nil {
			return nil, fmt.Errorf("marshaling %T: %w", val, err)
		}

		buf = bsoncore.AppendValueElement(buf, key, bsoncore.Value{
			Type: bsoncore.Type(bType),
			Data: raw,
		})
	}

	return buf, nil
}
