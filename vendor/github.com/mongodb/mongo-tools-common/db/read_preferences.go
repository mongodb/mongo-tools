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
	// default to primary if a read preference isn't specified anywhere
	if cmdReadPref == "" && (cs == nil || cs.ReadPreference == "") {
		return readpref.Primary(), nil
	}

	var doc readPrefDoc
	var err error
	if cmdReadPref != "" {
		doc, err = readPrefDocFromString(cmdReadPref)
	} else {
		doc = readPrefDocFromConnString(cs)
	}
	if err != nil {
		return nil, err
	}

	mode, err := readpref.ModeFromString(doc.Mode)
	if err != nil {
		return nil, err
	}

	tagSet := tag.NewTagSetFromMap(doc.Tags)
	return readpref.New(mode, readpref.WithTagSets(tagSet))
}

func readPrefDocFromConnString(cs *connstring.ConnString) readPrefDoc {
	doc := readPrefDoc{
		Mode: cs.ReadPreference,
	}

	if len(cs.ReadPreferenceTagSets) == 0 {
		// no tag sets
		return doc
	}

	// take the last tag set
	doc.Tags = cs.ReadPreferenceTagSets[len(cs.ReadPreferenceTagSets) - 1]
	return doc
}

func readPrefDocFromString(rp string) (readPrefDoc, error) {
	var doc readPrefDoc

	if rp[0] != '{' {
		doc.Mode = rp
		return doc, nil
	}

	err := json.Unmarshal([]byte(rp), &doc)
	if err != nil {
		return doc, fmt.Errorf("invalid --ReadPreferences json object: %v", err)
	}

	return doc, nil
}
