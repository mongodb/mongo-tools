package bsontools

import (
	"fmt"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

type alwaysMarshalableTypes interface {
	float64 | string | bson.Raw | bson.RawArray | bson.Binary |
		bson.Undefined | bson.ObjectID | bool | bson.DateTime | bson.Null |
		bson.Regex | bson.DBPointer | bson.JavaScript | bson.Symbol |

		// This type includes an `any`, which can’t always be marshaled.
		// bson.CodeWithScope |

		int32 | bson.Timestamp | int64 | bson.Decimal128 | bson.MinKey | bson.MaxKey |

		// convenience:
		int
}

type unmarshalTargets interface {
	alwaysMarshalableTypes |

		// This works for casting/unmarshaling but not for marshaling
		// (because we don’t have to parse a Go `any` value when casting).
		bson.CodeWithScope |

		// Convenience:
		time.Time
}

type cannotCastError struct {
	gotBSONType bson.Type
	toGoType    any
}

func (ce cannotCastError) Error() string {
	return fmt.Sprintf("cannot cast BSON %s to Go %T", ce.gotBSONType, ce.toGoType)
}

// RawValueTo is a bit like bson.UnmarshalValue, but it’s much faster because
// it avoids reflection. The downside is that only certain types are supported.
//
// This usually enforces strict numeric type equivalence. For example, it won’t
// coerce a float to an int64. If Go’s int type is the target, either int32 or
// int64 is acceptable.
//
// Example usage:
//
//	str, err := RawValueTo[string](rv)
func RawValueTo[T unmarshalTargets](in bson.RawValue) (T, error) {
	var zero T

	switch any(zero).(type) {
	case float64:
		if val, ok := in.DoubleOK(); ok {
			return any(val).(T), nil
		}
	case string:
		if str, ok := in.StringValueOK(); ok {
			return any(str).(T), nil
		}
	case bson.Raw:
		if doc, isDoc := in.DocumentOK(); isDoc {
			return any(doc).(T), nil
		}
	case bson.RawArray:
		if arr, ok := in.ArrayOK(); ok {
			return any(arr).(T), nil
		}
	case bson.Binary:
		if subtype, buf, ok := in.BinaryOK(); ok {
			return any(bson.Binary{
				Subtype: subtype,
				Data:    buf,
			}).(T), nil
		}
	case bson.Undefined:
		if in.Type == bson.TypeUndefined {
			return any(zero).(T), nil
		}
	case bson.ObjectID:
		if oid, ok := in.ObjectIDOK(); ok {
			return any(oid).(T), nil
		}
	case bool:
		if val, ok := in.BooleanOK(); ok {
			return any(val).(T), nil
		}
	case bson.DateTime:
		if i64, ok := in.DateTimeOK(); ok {
			return any(bson.DateTime(i64)).(T), nil
		}
	case bson.Null:
		if in.Type == bson.TypeNull {
			return any(zero).(T), nil
		}
	case bson.Regex:
		if pattern, opts, ok := in.RegexOK(); ok {
			return any(bson.Regex{
				Pattern: pattern,
				Options: opts,
			}).(T), nil
		}
	case bson.DBPointer:
		if db, ptr, ok := in.DBPointerOK(); ok {
			return any(bson.DBPointer{
				DB:      db,
				Pointer: ptr,
			}).(T), nil
		}
	case bson.JavaScript:
		if v, ok := in.JavaScriptOK(); ok {
			return any(bson.JavaScript(v)).(T), nil
		}
	case bson.Symbol:
		if v, ok := in.SymbolOK(); ok {
			return any(bson.Symbol(v)).(T), nil
		}
	case bson.CodeWithScope:
		if code, scope, ok := in.CodeWithScopeOK(); ok {
			return any(bson.CodeWithScope{
				Code:  bson.JavaScript(code),
				Scope: scope,
			}).(T), nil
		}
	case int32:
		if val, ok := in.Int32OK(); ok {
			return any(val).(T), nil
		}
	case bson.Timestamp:
		if t, i, ok := in.TimestampOK(); ok {
			return any(bson.Timestamp{T: t, I: i}).(T), nil
		}
	case int64:
		if val, ok := in.Int64OK(); ok {
			return any(val).(T), nil
		}
	case int:
		switch in.Type {
		case bson.TypeInt32, bson.TypeInt64:
			return any(int(in.AsInt64())).(T), nil
		}
	case bson.Decimal128:
		if dec, ok := in.Decimal128OK(); ok {
			return any(dec).(T), nil
		}
	case bson.MaxKey:
		if in.Type == bson.TypeMaxKey {
			return any(zero).(T), nil
		}
	case bson.MinKey:
		if in.Type == bson.TypeMinKey {
			return any(zero).(T), nil
		}

	// convenience:
	case time.Time:
		if val, ok := in.TimeOK(); ok {
			return any(val).(T), nil
		}
	default:
		panic(fmt.Sprintf("Unrecognized Go type: %T (missing case?)", zero))
	}

	return zero, cannotCastError{in.Type, zero}
}

// ToRawValue is a bit like bson.MarshalValue, but:
// - It’s faster since it avoids reflection.
// - It always succeeds since it only accepts certain known types.
func ToRawValue[T alwaysMarshalableTypes](in T) bson.RawValue {
	switch typedIn := any(in).(type) {
	case float64:
		return bson.RawValue{
			Type:  bson.TypeDouble,
			Value: bsoncore.AppendDouble(nil, typedIn),
		}
	case string:
		return bson.RawValue{
			Type:  bson.TypeString,
			Value: bsoncore.AppendString(nil, typedIn),
		}
	case bson.Raw:
		return bson.RawValue{
			Type:  bson.TypeEmbeddedDocument,
			Value: typedIn,
		}
	case bson.RawArray:
		return bson.RawValue{
			Type:  bson.TypeArray,
			Value: typedIn,
		}
	case bson.Binary:
		return bson.RawValue{
			Type:  bson.TypeBinary,
			Value: bsoncore.AppendBinary(nil, typedIn.Subtype, typedIn.Data),
		}
	case bson.Undefined:
		return bson.RawValue{
			Type: bson.TypeUndefined,
		}
	case bson.ObjectID:
		return bson.RawValue{
			Type:  bson.TypeObjectID,
			Value: bsoncore.AppendObjectID(nil, typedIn),
		}
	case bool:
		return bson.RawValue{
			Type:  bson.TypeBoolean,
			Value: bsoncore.AppendBoolean(nil, typedIn),
		}
	case bson.DateTime:
		return bson.RawValue{
			Type:  bson.TypeDateTime,
			Value: bsoncore.AppendDateTime(nil, int64(typedIn)),
		}
	case bson.Null:
		return bson.RawValue{
			Type: bson.TypeNull,
		}
	case bson.Regex:
		// To match the driver’s marshaling logic:
		opts := []rune(typedIn.Options)
		slices.Sort(opts)

		return bson.RawValue{
			Type:  bson.TypeRegex,
			Value: bsoncore.AppendRegex(nil, typedIn.Pattern, string(opts)),
		}
	case bson.DBPointer:
		return bson.RawValue{
			Type:  bson.TypeDBPointer,
			Value: bsoncore.AppendDBPointer(nil, typedIn.DB, typedIn.Pointer),
		}
	case bson.JavaScript:
		return bson.RawValue{
			Type:  bson.TypeJavaScript,
			Value: bsoncore.AppendJavaScript(nil, string(typedIn)),
		}
	case bson.Symbol:
		return bson.RawValue{
			Type:  bson.TypeSymbol,
			Value: bsoncore.AppendSymbol(nil, string(typedIn)),
		}
	case int32:
		return i32ToRawValue(typedIn)
	case bson.Timestamp:
		return bson.RawValue{
			Type:  bson.TypeTimestamp,
			Value: bsoncore.AppendTimestamp(nil, typedIn.T, typedIn.I),
		}
	case int64:
		return i64ToRawValue(typedIn)
	case bson.Decimal128:
		h, l := typedIn.GetBytes()

		return bson.RawValue{
			Type:  bson.TypeDecimal128,
			Value: bsoncore.AppendDecimal128(nil, h, l),
		}
	case bson.MaxKey:
		return bson.RawValue{
			Type: bson.TypeMaxKey,
		}
	case bson.MinKey:
		return bson.RawValue{
			Type: bson.TypeMinKey,
		}

	// For convenience:
	case int:
		if typedIn < math.MinInt32 || typedIn > math.MaxInt32 {
			return i64ToRawValue(int64(typedIn))
		}

		return i32ToRawValue(typedIn)
	}

	panic(fmt.Sprintf("Unrecognized Go type: %T (maybe add marshal instructions?)", in))
}

type i32Ish interface {
	int | int32
}

func i32ToRawValue[T i32Ish](in T) bson.RawValue {
	return bson.RawValue{
		Type:  bson.TypeInt32,
		Value: bsoncore.AppendInt32(nil, int32(in)),
	}
}

func i64ToRawValue(in int64) bson.RawValue {
	return bson.RawValue{
		Type:  bson.TypeInt64,
		Value: bsoncore.AppendInt64(nil, in),
	}
}
