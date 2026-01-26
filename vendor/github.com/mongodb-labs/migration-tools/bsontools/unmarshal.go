package bsontools

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// UnmarshalRaw mimics bson.Unmarshal to a bson.D.
func UnmarshalRaw(raw bson.Raw) (bson.D, error) {
	elsCount := 0

	for _, err := range RawElements(raw) {
		if err != nil {
			return nil, fmt.Errorf("parsing BSON: %w", err)
		}

		elsCount++
	}

	d := make(bson.D, 0, elsCount)

	for el, err := range RawElements(raw) {
		if err != nil {
			panic("parsing BSON (no error earlier?!?): " + err.Error())
		}

		key, err := el.KeyErr()
		if err != nil {
			return nil, fmt.Errorf("extracting field %d’s name: %w", len(d), err)
		}

		e := bson.E{}
		e.Key = key

		val, err := el.ValueErr()
		if err != nil {
			return nil, fmt.Errorf("extracting %#q value: %w", key, err)
		}

		e.Value, err = unmarshalValue(val)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling %#q value: %w", key, err)
		}

		d = append(d, e)
	}

	return d, nil
}

// UnmarshalArray is like UnmarshalRaw but for an array.
func UnmarshalArray(raw bson.RawArray) (bson.A, error) {
	elsCount := 0

	for _, err := range RawElements(bson.Raw(raw)) {
		if err != nil {
			return nil, fmt.Errorf("parsing BSON: %w", err)
		}

		elsCount++
	}

	a := make(bson.A, 0, elsCount)

	for el, err := range RawElements(bson.Raw(raw)) {
		if err != nil {
			panic("parsing BSON (no error earlier?!?): " + err.Error())
		}

		val, err := el.ValueErr()
		if err != nil {
			return nil, fmt.Errorf("extracting element %d: %w", len(a), err)
		}

		anyVal, err := unmarshalValue(val)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling element %d: %w", len(a), err)
		}

		a = append(a, anyVal)
	}

	return a, nil
}

func unmarshalValue(val bson.RawValue) (any, error) {
	switch val.Type {
	case bson.TypeDouble:
		return RawValueTo[float64](val)
	case bson.TypeString:
		return RawValueTo[string](val)
	case bson.TypeEmbeddedDocument:
		tVal, err := UnmarshalRaw(val.Value)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling subdoc: %w", err)
		}

		return tVal, nil
	case bson.TypeArray:
		tVal, err := UnmarshalArray(val.Value)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling array: %w", err)
		}

		return tVal, nil
	case bson.TypeBinary:
		return RawValueTo[bson.Binary](val)
	case bson.TypeUndefined:
		return bson.Undefined{}, nil
	case bson.TypeObjectID:
		return RawValueTo[bson.ObjectID](val)
	case bson.TypeBoolean:
		return RawValueTo[bool](val)
	case bson.TypeDateTime:
		return RawValueTo[bson.DateTime](val)
	case bson.TypeNull:
		return nil, nil
	case bson.TypeRegex:
		return RawValueTo[bson.Regex](val)
	case bson.TypeDBPointer:
		return RawValueTo[bson.DBPointer](val)
	case bson.TypeJavaScript:
		return RawValueTo[bson.JavaScript](val)
	case bson.TypeSymbol:
		return RawValueTo[bson.Symbol](val)
	case bson.TypeCodeWithScope:
		return RawValueTo[bson.CodeWithScope](val)
	case bson.TypeInt32:
		return RawValueTo[int32](val)
	case bson.TypeTimestamp:
		return RawValueTo[bson.Timestamp](val)
	case bson.TypeInt64:
		return RawValueTo[int64](val)
	case bson.TypeDecimal128:
		return RawValueTo[bson.Decimal128](val)
	case bson.TypeMaxKey:
		return bson.MaxKey{}, nil
	case bson.TypeMinKey:
		return bson.MinKey{}, nil
	default:
		panic(fmt.Sprintf("unknown BSON type: %d", val.Type))
	}
}
