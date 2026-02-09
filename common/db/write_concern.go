// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"fmt"
	"strconv"
	"time"

	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// write concern fields.
const (
	j         = "j"
	w         = "w"
	wTimeout  = "wtimeout"
	majString = "majority"
)

// NewMongoWriteConcern takes a string (from the command line writeConcern option) and a ConnString object
// (from the command line uri option) and returns a WriteConcern. If both are provided, preference is given to
// the command line writeConcern option. If neither is provided, the default 'majority' write concern is constructed.
func NewMongoWriteConcern(
	writeConcern string,
	cs *connstring.ConnString,
) (wc *wcwrapper.WriteConcern, err error) {

	// Log whatever write concern was generated
	defer func() {
		if wc != nil {
			log.Logvf(log.Info, "using write concern: %v", wc)
		}
	}()

	// URI Connection String provided but no String provided case; constructWCFromConnString handles
	// default for ConnString without write concern
	if writeConcern == "" && cs != nil {
		return constructWCFromConnString(cs)
	}

	// String case; constructWCFromString handles default for empty string
	return constructWCFromString(writeConcern)
}

// constructWCFromConnString takes in a parsed connection string and
// extracts values from it. If the ConnString has no write concern value, it defaults
// to 'majority'.
func constructWCFromConnString(cs *connstring.ConnString) (*wcwrapper.WriteConcern, error) {
	wc := wcwrapper.New()

	switch {
	case cs.WNumberSet:
		if cs.WNumber < 0 {
			return nil, fmt.Errorf("invalid 'w' argument: %v", cs.WNumber)
		}

		wc.W = cs.WNumber
	case cs.WString != "":
		wc.W = cs.WString
	default:
		wc.W = writeconcern.WCMajority
	}

	if cs.J {
		wc.Journal = &cs.J
	}

	// We cannot set WTimeout from the connstring, because the driver v2 connstring package won't
	// parse them.

	return wc, nil
}

// constructWCFromString takes in a write concern and attempts to
// extract values from it. It returns an error if it is unable to parse the
// string or if a parsed write concern field value is invalid.
func constructWCFromString(writeConcern string) (*wcwrapper.WriteConcern, error) {

	// Default case
	if writeConcern == "" {
		return wcwrapper.Majority(), nil
	}

	// Try to unmarshal as JSON document
	jsonWriteConcern := map[string]interface{}{}
	err := json.Unmarshal([]byte(writeConcern), &jsonWriteConcern)
	if err == nil {
		return parseJSONWriteConcern(jsonWriteConcern)
	}

	// If JSON parsing fails, try to parse it as a plain string instead.  This
	// allows a default to the old behavior wherein the entire argument passed
	// in is assigned to the 'w' field - thus allowing users pass a write
	// concern that looks like: "majority", 0, "4", etc.
	wOpt, err := parseModeString(writeConcern)
	if err != nil {
		return nil, err
	}

	return wcwrapper.Wrap(&writeconcern.WriteConcern{W: wOpt}), nil
}

// parseJSONWriteConcern converts a JSON map representing a write concern object into a WriteConcern.
func parseJSONWriteConcern(
	jsonWriteConcern map[string]interface{},
) (*wcwrapper.WriteConcern, error) {
	wc := wcwrapper.New()

	// Construct new options from 'w', if it exists; otherwise default to 'majority'
	if wVal, ok := jsonWriteConcern[w]; ok {
		rawW, err := parseWField(wVal)
		if err != nil {
			return nil, err
		}

		wc.W = rawW
	} else {
		wc.W = writeconcern.WCMajority
	}

	// Journal option
	if jVal, ok := jsonWriteConcern[j]; ok && util.IsTruthy(jVal) {
		wc.Journal = lo.ToPtr(true)
	}

	// Wtimeout option
	if wtimeout, ok := jsonWriteConcern[wTimeout]; ok {
		timeoutVal, err := util.ToInt(wtimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid '%v' argument: %v", wTimeout, wtimeout)
		}
		// Previous implementation assumed passed in string was milliseconds
		wc.WTimeout = time.Duration(timeoutVal) * time.Millisecond
	}

	return wc, nil
}

func parseWField(wValue interface{}) (any, error) {
	// Try parsing as int
	if wNumber, err := util.ToInt(wValue); err == nil {
		return parseModeNumber(wNumber)
	}

	// Try parsing as string
	if wStrVal, ok := wValue.(string); ok {
		return parseModeString(wStrVal)
	}

	return nil, fmt.Errorf("invalid 'w' argument type: %v has type %T", wValue, wValue)
}

// Given an integer, returns a write concern object or error.
func parseModeNumber(wNumber int) (any, error) {
	if wNumber < 0 {
		return nil, fmt.Errorf("invalid 'w' argument: %v", wNumber)
	}

	return wNumber, nil
}

// Given a string, returns a write concern object or error.
func parseModeString(wString string) (any, error) {
	// Default case
	if wString == "" {
		return writeconcern.WCMajority, nil
	}

	// Try parsing as number before treating as just a string
	if wNumber, err := strconv.Atoi(wString); err == nil {
		return parseModeNumber(wNumber)
	}

	return wString, nil
}
