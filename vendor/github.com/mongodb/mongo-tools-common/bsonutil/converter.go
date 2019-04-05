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

	"github.com/mongodb/mongo-tools-common/json"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ConvertJSONValueToBSON walks through a document or an array and
// replaces any extended JSON value with its corresponding BSON type.
func ConvertJSONValueToBSON(x interface{}) (interface{}, error) {
	switch v := x.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil
	case map[string]interface{}: // document
		for key, jsonValue := range v {
			bsonValue, err := ParseJSONValue(jsonValue)
			if err != nil {
				return nil, err
			}
			v[key] = bsonValue
		}
		return v, nil
	case bson.D:
		for i := range v {
			var err error
			v[i].Value, err = ParseJSONValue(v[i].Value)
			if err != nil {
				return nil, err
			}
		}
		return v, nil

	case []interface{}: // array
		for i, jsonValue := range v {
			bsonValue, err := ParseJSONValue(jsonValue)
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
		return primitive.ObjectIDFromHex(s)

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
		return primitive.Binary{v.Type, data}, nil

	case json.DBPointer: // DBPointer, for backwards compatibility
		return primitive.DBPointer{v.Namespace, v.Id}, nil

	case json.RegExp: // RegExp
		return primitive.Regex{v.Pattern, v.Options}, nil

	case json.Timestamp: // Timestamp
		return primitive.Timestamp{T: v.Seconds, I: v.Increment}, nil

	case json.JavaScript: // Javascript
		return primitive.CodeWithScope{Code: primitive.JavaScript(v.Code), Scope: v.Scope}, nil

	case json.MinKey: // MinKey
		return primitive.MinKey{}, nil

	case json.MaxKey: // MaxKey
		return primitive.MaxKey{}, nil

	case json.Undefined: // undefined
		return primitive.Undefined{}, nil

	default:
		return nil, fmt.Errorf("conversion of JSON value '%v' of type '%T' not supported", v, v)
	}
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

func getConvertedKeys(v bson.M) (bson.M, error) {
	out := bson.M{}
	for key, value := range v {
		jsonValue, err := GetBSONValueAsJSON(value)
		if err != nil {
			return nil, err
		}
		out[key] = jsonValue
	}
	return out, nil
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
	case bson.D:
		for i, value := range v {
			jsonValue, err := ConvertBSONValueToJSON(value.Value)
			if err != nil {
				return nil, err
			}
			v[i].Value = jsonValue
		}
		return MarshalD(v), nil
	case MarshalD:
		return v, nil
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

	case primitive.ObjectID: // ObjectId
		return json.ObjectId(v.Hex()), nil

	case primitive.Decimal128:
		return json.Decimal128{v}, nil

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

	case primitive.Binary: // BinData
		data := base64.StdEncoding.EncodeToString(v.Data)
		return json.BinData{v.Subtype, data}, nil

	case primitive.DBPointer: // DBPointer
		return json.DBPointer{v.DB, v.Pointer}, nil

	case primitive.Regex: // RegExp
		return json.RegExp{v.Pattern, v.Options}, nil

	case primitive.Timestamp: // Timestamp
		return json.Timestamp{
			Seconds:   v.T,
			Increment: v.I,
		}, nil

	case primitive.CodeWithScope: // JavaScript
		var scope interface{}
		var err error
		if v.Scope != nil {
			scope, err = ConvertBSONValueToJSON(v.Scope)
			if err != nil {
				return nil, err
			}
		}
		return json.JavaScript{string(v.Code), scope}, nil

	default:
		switch x {
		case primitive.MinKey{}: // MinKey
			return json.MinKey{}, nil

		case primitive.MaxKey{}: // MaxKey
			return json.MaxKey{}, nil

		case primitive.Undefined{}: // undefined
			return json.Undefined{}, nil
		}
	}

	return nil, fmt.Errorf("conversion of BSON value '%v' of type '%T' not supported", x, x)
}

// GetBSONValueAsJSON is equivalent to ConvertBSONValueToJSON, but does not mutate its argument.
func GetBSONValueAsJSON(x interface{}) (interface{}, error) {
	switch v := x.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil

	case *bson.M: // document
		doc, err := getConvertedKeys(*v)
		if err != nil {
			return nil, err
		}
		return doc, err
	case bson.M: // document
		return getConvertedKeys(v)
	case map[string]interface{}:
		return getConvertedKeys(v)
	case bson.D:
		out := bson.D{}
		for _, value := range v {
			jsonValue, err := GetBSONValueAsJSON(value.Value)
			if err != nil {
				return nil, err
			}
			out = append(out, bson.E{
				Key:  value.Key,
				Value: jsonValue,
			})
		}
		return MarshalD(out), nil
	case MarshalD:
		out, err := GetBSONValueAsJSON(bson.D(v))
		if err != nil {
			return nil, err
		}
		return MarshalD(out.(bson.D)), nil
	case []interface{}: // array
		out := []interface{}{}
		for _, value := range v {
			jsonValue, err := GetBSONValueAsJSON(value)
			if err != nil {
				return nil, err
			}
			out = append(out, jsonValue)
		}
		return out, nil

	case string:
		return v, nil // require no conversion

	case int:
		return json.NumberInt(v), nil

	case primitive.ObjectID: // ObjectId
		return json.ObjectId(v.Hex()), nil

	case primitive.Decimal128:
		return json.Decimal128{v}, nil

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

	case primitive.Binary: // BinData
		data := base64.StdEncoding.EncodeToString(v.Data)
		return json.BinData{Type: v.Subtype, Base64: data}, nil

	case primitive.DBPointer: // DBPointer
		return json.DBPointer{v.DB, v.Pointer}, nil

	case primitive.Regex: // RegExp
		return json.RegExp{v.Pattern, v.Options}, nil

	case primitive.Timestamp: // Timestamp
		return json.Timestamp{
			Seconds:   v.T,
			Increment: v.I,
		}, nil

	case primitive.CodeWithScope: // JavaScript
		var scope interface{}
		var err error
		if v.Scope != nil {
			scope, err = GetBSONValueAsJSON(v.Scope)
			if err != nil {
				return nil, err
			}
		}
		return json.JavaScript{string(v.Code), scope}, nil

	default:
		switch x {
		case primitive.MinKey{}: // MinKey
			return json.MinKey{}, nil

		case primitive.MaxKey{}: // MaxKey
			return json.MaxKey{}, nil

		case primitive.Undefined{}: // undefined
			return json.Undefined{}, nil
		}
	}

	return nil, fmt.Errorf("conversion of BSON value '%v' of type '%T' not supported", x, x)
}
