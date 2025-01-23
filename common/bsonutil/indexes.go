package bsonutil

import (
	"math"
	"math/big"

	"github.com/mongodb/mongo-tools/common/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// validIndexOptions are taken from https://github.com/mongodb/mongo/blob/master/src/mongo/db/index/index_descriptor.h
var validIndexOptions = map[string]bool{
	"2dsphereIndexVersion":    true,
	"background":              true,
	"bits":                    true,
	"bucketSize":              true,
	"coarsestIndexedLevel":    true,
	"collation":               true,
	"default_language":        true,
	"expireAfterSeconds":      true,
	"finestIndexedLevel":      true,
	"key":                     true,
	"language_override":       true,
	"max":                     true,
	"min":                     true,
	"name":                    true,
	"ns":                      true,
	"partialFilterExpression": true,
	"sparse":                  true,
	"storageEngine":           true,
	"textIndexVersion":        true,
	"unique":                  true,
	"v":                       true,
	"weights":                 true,
	"wildcardProjection":      true,
}

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

// ConvertLegacyIndexKeys transforms the values of index definitions pre 3.4 into
// the stricter index definitions of 3.4+. Prior to 3.4, any value in an index key
// that isn't a negative number or that isn't a string is treated as int32(1).
// The one exception is an empty string is treated as int32(1).
// All other strings that aren't one of ["2d", "geoHaystack", "2dsphere", "hashed", "text", ""]
// will cause the index build to fail. See TOOLS-2412 for more information.
//
// This function logs the keys that are converted.
func ConvertLegacyIndexKeys(indexKey bson.D, ns string) {
	var converted bool
	originalJSONString := CreateExtJSONString(indexKey)
	for j, elem := range indexKey {
		switch v := elem.Value.(type) {
		case int:
			if v == 0 {
				indexKey[j].Value = int32(1)
				converted = true
			}
		case int32:
			if v == int32(0) {
				indexKey[j].Value = int32(1)
				converted = true
			}
		case int64:
			if v == int64(0) {
				indexKey[j].Value = int32(1)
				converted = true
			}
		case float64:
			if math.Abs(v-float64(0)) < epsilon {
				indexKey[j].Value = int32(1)
				converted = true
			}
		case primitive.Decimal128:
			if bi, _, err := v.BigInt(); err == nil {
				if bi.Cmp(big.NewInt(0)) == 0 {
					indexKey[j].Value = int32(1)
					converted = true
				}
			}
		case string:
			// Only convert an empty string
			if v == "" {
				indexKey[j].Value = int32(1)
				converted = true
			}
		default:
			// Convert all types that aren't strings or numbers
			indexKey[j].Value = int32(1)
			converted = true
		}
	}
	if converted {
		newJSONString := CreateExtJSONString(indexKey)
		log.Logvf(
			log.Always,
			"convertLegacyIndexes: converted index values '%s' to '%s' on collection '%s'",
			originalJSONString,
			newJSONString,
			ns,
		)
	}
}

// ConvertLegacyIndexOptions removes options that don't match a known list of index options.
// It is preferable to use the ignoreUnknownIndexOptions on the createIndex command to
// force the server to do this task. But that option was only added in 4.1.9. So for
// pre 3.4 indexes being added to servers 3.4 - 4.2, we must strip the options in the client.
// This function processes the indexes Options inside collection dump.
func ConvertLegacyIndexOptions(indexOptions bson.M) {
	var converted bool
	originalJSONString := CreateExtJSONString(indexOptions)
	for key := range indexOptions {
		if _, ok := validIndexOptions[key]; !ok {
			delete(indexOptions, key)
			converted = true
		}
	}
	if converted {
		newJSONString := CreateExtJSONString(indexOptions)
		log.Logvf(log.Always, "convertLegacyIndexes: converted index options '%s' to '%s'",
			originalJSONString, newJSONString)
	}
}

// ConvertLegacyIndexOptionsFromOp removes options that don't match a known list of index options.
// It is preferable to use the ignoreUnknownIndexOptions on the createIndex command to
// force the server to do this task. But that option was only added in 4.1.9. So for
// pre 3.4 indexes being added to servers 3.4 - 4.2, we must strip the options in the client.
// This function processes the index options inside createIndexes command.
func ConvertLegacyIndexOptionsFromOp(indexOptions *bson.D) {
	var converted bool
	originalJSONString := CreateExtJSONString(indexOptions)
	var newIndexOptions bson.D

	for i, elem := range *indexOptions {
		if _, ok := validIndexOptions[elem.Key]; !ok && elem.Key != "createIndexes" {
			// Remove this key.
			converted = true
		} else {
			newIndexOptions = append(newIndexOptions, (*indexOptions)[i])
		}
	}
	if converted {
		*indexOptions = newIndexOptions
		newJSONString := CreateExtJSONString(newIndexOptions)
		log.Logvf(
			log.Always,
			"ConvertLegacyIndexOptionsFromOp: converted index options '%s' to '%s'",
			originalJSONString,
			newJSONString,
		)
	}
}

// CreateExtJSONString stringifies doc as Extended JSON. It does not error
// if it's unable to marshal the doc to JSON.
func CreateExtJSONString(doc interface{}) string {
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
