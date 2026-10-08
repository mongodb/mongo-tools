package bsonutil

import (
	"math"
	"math/big"

	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const epsilon = 1e-9

func IsIndexKeysEqual(indexKey1 bson.D, indexKey2 bson.D) bool {
	if len(indexKey1) != len(indexKey2) {
		// two indexes have different number of keys
		return false
	}

	for j, elem := range indexKey1 {
		if elem.Key != indexKey2[j].Key {
			return false
		}

		// After ConvertLegacyIndexKeys, index key value should only be numerical or string value
		switch key1Value := elem.Value.(type) {
		case string:
			if key2Value, ok := indexKey2[j].Value.(string); ok {
				if key1Value == key2Value {
					continue
				}
			}
			return false
		default:
			if key1Value, ok := Bson2Float64(key1Value); ok {
				if key2Value, ok := Bson2Float64(indexKey2[j].Value); ok {
					if math.Abs(key1Value-key2Value) < epsilon {
						continue
					}
				}
			}
			return false
		}
	}
	return true
}

// ConvertLegacyIndexKeyValue returns the normalized value and a boolean that indicates whether the
// value was normalized/converted.
func ConvertLegacyIndexKeyValue(value any) (any, bool) {
	switch v := value.(type) {
	case int:
		if v == 0 {
			return int32(1), true
		}
	case int32:
		if v == int32(0) {
			return int32(1), true
		}
	case int64:
		if v == int64(0) {
			return int32(1), true
		}
	case float64:
		if math.Abs(v) < epsilon {
			return lo.Ternary[int32](v >= 0, 1, -1), true
		}
	case bson.Decimal128:
		if bi, _, err := v.BigInt(); err == nil {
			if bi.Cmp(big.NewInt(0)) == 0 {
				return int32(1), true
			}
		}
	case string:
		// Only convert an empty string
		if v == "" {
			return int32(1), true
		}
	default:
		// Convert all types that aren't strings or numbers
		return int32(1), true
	}

	return value, false
}

// CreateExtJSONString stringifies doc as Extended JSON. It does not error
// if it's unable to marshal the doc to JSON.
func CreateExtJSONString(doc any) string {
	// by default return "<unable to format document>"" since we don't
	// want to throw an error when formatting informational messages.
	// An error would be inconsequential.
	JSONString := "<unable to format document>"
	JSONBytes, err := MarshalExtJSONReversible(doc, false, false)
	if err == nil {
		JSONString = string(JSONBytes)
	}
	return JSONString
}
