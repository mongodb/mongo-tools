// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package idx

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
)

// IndexDocument holds information about a collection's index.
type IndexDocument struct {
	Options                 bson.M `bson:",inline"`
	Key                     bson.D `bson:"key"`
	PartialFilterExpression bson.D `bson:"partialFilterExpression,omitempty"`
}

// NewIndexDocumentFromD converts a bson.D index spec into an IndexDocument
func NewIndexDocumentFromD(doc bson.D) (*IndexDocument, error) {
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
