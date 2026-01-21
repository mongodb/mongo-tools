// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package idx

import (
	"fmt"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/util"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// IndexDocument holds information about a collection's index.
type IndexDocument struct {
	Options                 bson.M `bson:",inline"`
	Key                     bson.D `bson:"key"`
	PartialFilterExpression bson.D `bson:"partialFilterExpression,omitempty"`
}

// newIndexDocumentFromD converts a bson.D index spec into an IndexDocument. This is only used in
// test code.
func newIndexDocumentFromD(doc bson.D) (*IndexDocument, error) {
	indexDoc := IndexDocument{}

	for _, elem := range doc {
		switch elem.Key {
		case "key":
			if val, ok := elem.Value.(bson.D); ok {
				indexDoc.Key = val
				continue
			} else {
				return nil, fmt.Errorf("index key could not type assert to bson.D")
			}
		case "partialFilterExpression":
			if val, ok := elem.Value.(bson.D); ok {
				indexDoc.PartialFilterExpression = val
				continue
			} else {
				return nil, fmt.Errorf("index partialFilterExpression could not type assert to bson.D")
			}
		default:
			indexDoc.Options[elem.Key] = elem.Value
		}
	}

	return &indexDoc, nil
}

// IsDefaultIdIndex indicates whether the IndexDocument represents its
// collection’s default _id index.
func (id *IndexDocument) IsDefaultIdIndex() bool {

	// Default indexes can’t have partial filters.
	if id.PartialFilterExpression != nil {
		return false
	}

	indexKeyIsIdOnly := len(id.Key) == 1 && id.Key[0].Key == "_id"

	if !indexKeyIsIdOnly {
		return false
	}

	// We need to ignore special indexes like hashed or 2dsphere. Historically
	// “non-special” indexes weren’t always persisted with 1 as the value,
	// so before we check for “special” we normalize.
	normalizedVal, _ := bsonutil.ConvertLegacyIndexKeyValue(id.Key[0].Value)

	// Default indexes are always { _id:1 }. They’re probably always int32(1),
	// but let’s be more permissive than that.
	normalizedAsF64, err := util.ToFloat64(normalizedVal)

	// An error here just means that the value can‘t be cast to a float64
	// (e.g., is a string).
	if err != nil {
		return false
	}

	return normalizedAsF64 == 1
}
