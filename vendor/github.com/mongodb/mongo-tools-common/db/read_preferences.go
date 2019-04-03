// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"fmt"

	"github.com/mongodb/mongo-tools-common/json"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/tag"
	"go.mongodb.org/mongo-driver/x/network/connstring"
)

type readPrefDoc struct {
	Mode string
	Tags map[string]string
}

const (
	WarningNonPrimaryMongosConnection = "Warning: using a non-primary readPreference with a " +
		"connection to mongos may produce inconsistent duplicates or miss some documents."
)

// NewReadPreference takes a string (command line read preference argument) and a ConnString (from the command line
// URI argument) and returns a ReadPref. If both are provided, preference is given to the command line argument. If
// both are empty, a default read preference of primary will be returned.
func NewReadPreference(cmdReadPref string, cs *connstring.ConnString) (*readpref.ReadPref, error) {
	var rp string
	if cs != nil {
		rp = cs.ReadPreference
	}
	if cmdReadPref != "" {
		rp = cmdReadPref
	}

	var mode string
	var tagSet tag.Set
	if rp == "" {
		return readpref.Primary(), nil
	}
	if rp[0] != '{' {
		mode = rp
	} else {
		var doc readPrefDoc
		err := json.Unmarshal([]byte(rp), &doc)
		if err != nil {
			return nil, fmt.Errorf("invalid --ReadPreferences json object: %v", err)
		}
		tagSet = tag.NewTagSetFromMap(doc.Tags)
		mode = doc.Mode
	}
	switch mode {
	case "primary":
		return readpref.Primary(), nil
	case "primaryPreferred":
		return readpref.PrimaryPreferred(readpref.WithTagSets(tagSet)), nil
	case "secondary":
		return readpref.Secondary(readpref.WithTagSets(tagSet)), nil
	case "secondaryPreferred":
		return readpref.SecondaryPreferred(readpref.WithTagSets(tagSet)), nil
	case "nearest":
		return readpref.Nearest(readpref.WithTagSets(tagSet)), nil
	}
	return nil, fmt.Errorf("invalid readPreference mode '%v'", mode)
}
