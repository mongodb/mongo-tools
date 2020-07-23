package bsonutil

import (
	"github.com/mongodb/mongo-tools-common/log"
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

// ConvertLegacyIndexKeys transforms the values of index definitions pre 3.4 into
// the stricter index definitions of 3.4+. Prior to 3.4, any value in an index key
// that isn't a negative number or that isn't a string is treated as 1.
// The one exception is an empty string is treated as 1.
// All other strings that aren't one of ["2d", "geoHaystack", "2dsphere", "hashed", "text", ""]
// will cause the index build to fail. See TOOLS-2412 for more information.
//
// This function logs the keys that are converted.
func ConvertLegacyIndexKeys(indexKey bson.D, ns string) {
	var converted bool
	originalJSONString := CreateExtJSONString(indexKey)
	for j, elem := range indexKey {
		indexVal := 1
		needsConversion := false
		switch v := elem.Value.(type) {
		case int32:
			indexVal = int(v)
			needsConversion = true
		case int64:
			indexVal = int(v)
			needsConversion = true
		case float64:
			indexVal = int(v)
			needsConversion = true
		case primitive.Decimal128:
			if intVal, _, err := v.BigInt(); err == nil {
				indexVal = int(intVal.Int64())

				needsConversion = true
			}
		case string:
			if v == "" {
				indexKey[j].Value = 1
				converted = true
			}
		default:
			// Convert all types that aren't strings or numbers
			indexKey[j].Value = 1
			converted = true
		}
		if needsConversion {
			if indexVal < 0 {
				indexKey[j].Value = -1
			} else {
				indexKey[j].Value = 1
			}
			converted = true
		}
	}
	if converted {
		newJSONString := CreateExtJSONString(indexKey)
		log.Logvf(log.Always, "convertLegacyIndexes: converted index values '%s' to '%s' on collection '%s'",
			originalJSONString, newJSONString, ns)
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
	doc := *indexOptions
	originalJSONString := CreateExtJSONString(indexOptions)
	for i, elem := range *indexOptions {
		if _, ok := validIndexOptions[elem.Key]; !ok && elem.Key != "createIndexes" {
			// Remove this key.
			*indexOptions = append(doc[:i], doc[i+1:]...)
			converted = true
		}
	}
	if converted {
		newJSONString := CreateExtJSONString(indexOptions)
		log.Logvf(log.Always, "ConvertLegacyIndexOptionsFromOp: converted index options '%s' to '%s'",
			originalJSONString, newJSONString)
	}
}

// CreateExtJSONString stringifies doc as Extended JSON. It does not error
// if it's unable to marshal the doc to JSON.
func CreateExtJSONString(doc interface{}) string {
	// by default return "<unable to format document>"" since we don't
	// want to throw an error when formatting informational messages.
	// An error would be inconsequential.
	JSONString := "<unable to format document>"
	JSONBytes, err := bson.MarshalExtJSON(doc, false, false)
	if err == nil {
		JSONString = string(JSONBytes)
	}
	return JSONString
}