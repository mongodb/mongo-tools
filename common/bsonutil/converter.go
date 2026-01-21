// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/util"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ConvertLegacyExtJSONValueToBSON walks through a document or an array and
// replaces any extended JSON value with its corresponding BSON type.
func ConvertLegacyExtJSONValueToBSON(x interface{}) (interface{}, error) {
	switch v := x.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil
	case map[string]interface{}: // document
		for key, jsonValue := range v {
			bsonValue, err := ParseLegacyExtJSONValue(jsonValue)
			if err != nil {
				return nil, err
			}
			v[key] = bsonValue
		}
		return v, nil
	case bson.D:
		for i := range v {
			var err error
			v[i].Value, err = ParseLegacyExtJSONValue(v[i].Value)
			if err != nil {
				return nil, err
			}
		}
		return v, nil

	case []interface{}: // array
		for i, jsonValue := range v {
			bsonValue, err := ParseLegacyExtJSONValue(jsonValue)
			if err != nil {
				return nil, err
			}
			v[i] = bsonValue
		}
		return v, nil

	case string, float64, int32, int64:
		return v, nil // require no conversion

	case json.ObjectId: // ObjectId
		s := string(v)
		return bson.ObjectIDFromHex(s)

	case json.Decimal128:
		return v.Decimal128, nil

	case json.Date: // Date
		n := int64(v)
		return time.Unix(n/1e3, n%1e3*1e6), nil

	case json.ISODate: // ISODate
		n := string(v)
		return util.FormatDate(n)

	case json.NumberLong: // NumberLong
		return int64(v), nil

	case json.NumberInt: // NumberInt
		return int32(v), nil

	case json.NumberFloat: // NumberFloat
		return float64(v), nil
	case json.BinData: // BinData
		data, err := base64.StdEncoding.DecodeString(v.Base64)
		if err != nil {
			return nil, err
		}
		return bson.Binary{v.Type, data}, nil

	case json.DBPointer: // DBPointer, for backwards compatibility
		return bson.DBPointer{v.Namespace, v.Id}, nil

	case json.RegExp: // RegExp
		return bson.Regex{v.Pattern, v.Options}, nil

	case json.Timestamp: // Timestamp
		return bson.Timestamp{T: v.Seconds, I: v.Increment}, nil

	case json.JavaScript: // Javascript
		if v.Scope != nil {
			return bson.CodeWithScope{Code: bson.JavaScript(v.Code), Scope: v.Scope}, nil
		}
		return bson.JavaScript(v.Code), nil

	case json.MinKey: // MinKey
		return bson.MinKey{}, nil

	case json.MaxKey: // MaxKey
		return bson.MaxKey{}, nil

	case json.Undefined: // undefined
		return bson.Undefined{}, nil

	default:
		return nil, fmt.Errorf("conversion of JSON value '%v' of type '%T' not supported", v, v)
	}
}

func convertKeys(v bson.M) (bson.M, error) {
	for key, value := range v {
		jsonValue, err := ConvertBSONValueToLegacyExtJSON(value)
		if err != nil {
			return nil, err
		}
		v[key] = jsonValue
	}
	return v, nil
}

func convertArray(v bson.A) ([]interface{}, error) {
	for i, value := range v {
		jsonValue, err := ConvertBSONValueToLegacyExtJSON(value)
		if err != nil {
			return nil, err
		}
		v[i] = jsonValue
	}
	return []interface{}(v), nil
}

// ConvertBSONValueToLegacyExtJSON walks through a document or an array and
// converts any BSON value to its corresponding extended JSON type.
// It returns the converted JSON document and any error encountered.
func ConvertBSONValueToLegacyExtJSON(x interface{}) (interface{}, error) {
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
	case bson.D:
		for i, value := range v {
			jsonValue, err := ConvertBSONValueToLegacyExtJSON(value.Value)
			if err != nil {
				return nil, err
			}
			v[i].Value = jsonValue
		}
		return MarshalD(v), nil
	case MarshalD:
		return v, nil
	case bson.A: // array
		return convertArray(v)
	case []interface{}: // array
		return convertArray(v)
	case string:
		return v, nil // require no conversion

	case int:
		return safecast.Convert[json.NumberInt](v)

	case bson.ObjectID: // ObjectId
		return json.ObjectId(v.Hex()), nil

	case bson.Decimal128:
		return json.Decimal128{v}, nil

	case bson.DateTime: // Date
		return json.Date(v), nil

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
		return json.BinData{v.Subtype, data}, nil

	case bson.DBPointer: // DBPointer
		return json.DBPointer{v.DB, v.Pointer}, nil

	case bson.Regex: // RegExp
		return json.RegExp{v.Pattern, v.Options}, nil

	case bson.Timestamp: // Timestamp
		return json.Timestamp{
			Seconds:   v.T,
			Increment: v.I,
		}, nil

	case bson.JavaScript: // JavaScript Code
		return json.JavaScript{Code: string(v), Scope: nil}, nil

	case bson.CodeWithScope: // JavaScript Code w/ Scope
		var scope interface{}
		var err error
		if v.Scope != nil {
			scope, err = ConvertBSONValueToLegacyExtJSON(v.Scope)
			if err != nil {
				return nil, err
			}
		}
		return json.JavaScript{string(v.Code), scope}, nil

	case bson.MaxKey: // MaxKey
		return json.MaxKey{}, nil

	case bson.MinKey: // MinKey
		return json.MinKey{}, nil

	case bson.Undefined: // undefined
		return json.Undefined{}, nil

	case bson.Null: // Null
		return nil, nil
	}

	return nil, fmt.Errorf("conversion of BSON value '%v' of type '%T' not supported", x, x)
}
