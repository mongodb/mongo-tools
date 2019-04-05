// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"bytes"
	"fmt"

	"github.com/mongodb/mongo-tools-common/json"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/bson"
)

// MarshalD is a wrapper for bson.D that allows unmarshalling
// of bson.D with preserved order. Necessary for printing
// certain database commands.
type MarshalD bson.D

// MarshalJSON makes the MarshalD type usable by
// the encoding/json package.
func (md MarshalD) MarshalJSON() ([]byte, error) {
	var buff bytes.Buffer
	buff.WriteString("{")
	for i, item := range md {
		key, err := json.Marshal(item.Key)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal key %v: %v", item.Key, err)
		}
		val, err := json.Marshal(item.Value)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal value %v: %v", item.Value, err)
		}
		buff.Write(key)
		buff.WriteString(":")
		buff.Write(val)
		if i != len(md)-1 {
			buff.WriteString(",")
		}
	}
	buff.WriteString("}")
	return buff.Bytes(), nil
}

// MakeSortString takes a bson.D object and converts it to a slice of strings
// that can be used as the input args to mgo's .Sort(...) function.
// For example:
// {a:1, b:-1} -> ["+a", "-b"]
func MakeSortString(sortObj bson.D) ([]string, error) {
	sortStrs := make([]string, 0, len(sortObj))
	for _, docElem := range sortObj {
		valueAsNumber, err := util.ToFloat64(docElem.Value)
		if err != nil {
			return nil, err
		}
		prefix := "+"
		if valueAsNumber < 0 {
			prefix = "-"
		}
		sortStrs = append(sortStrs, fmt.Sprintf("%v%v", prefix, docElem.Key))
	}
	return sortStrs, nil
}
