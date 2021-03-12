// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"go.mongodb.org/mongo-driver/bson"
)

// IndexDocument holds information about a collection's index.
type IndexDocument struct {
	Options                 bson.M `bson:",inline"`
	Key                     bson.D `bson:"key"`
	PartialFilterExpression bson.D `bson:"partialFilterExpression,omitempty"`
}
