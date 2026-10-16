// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import "go.mongodb.org/mongo-driver/v2/bson"

const (
	mioSoeFile     = "testdata/10k1dup10k.bson"
	longFilePrefix = "aVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery" +
		"VeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery" +
		"VeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVeryVery"
	longCollectionName = longFilePrefix +
		"LongCollectionNameConsistingOfExactlyTwoHundredAndFortySevenCharacters"
	longBsonName = longFilePrefix +
		"LongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.bson"
	longMetadataName = longFilePrefix +
		"LongCollectionNameConsistingOfE%24xFO0VquRn7cg3QooSZD5sglTddU.metadata.json"
	longInvalidBson = longFilePrefix +
		"LongCollectionNameConsistingOfE%24someMadeUpInvalidHashString.bson"
	specialCharactersCollectionName = "cafés"
)

var testDocument = bson.M{"key": "value"}
