// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"github.com/mongodb/mongo-tools-common/json"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/x/network/connstring"

	"fmt"
	"strconv"
	"time"
)

// write concern fields
const (
	j         = "j"
	w         = "w"
	fSync     = "fsync"
	wTimeout  = "wtimeout"
	majString = "majority"
	tagString = "tagset"
)

// WriteConcernOptions is a shim in between write concern option configuration
// inputs and the Go driver write concern option type, which is currently
// opaque (GODRIVER-752).  Having the shim allows easier testing of
// configuration parsing.
type WriteConcernOptions struct {
	WNumber    int
	WNumberSet bool
	WString    string
	J          bool
	WTimeout   time.Duration
}

// NewMongoWriteConcernOpts takes a string (from the command line writeConcern option) and a ConnString object
// (from the command line uri option) and returns a WriteConcernOptions. If both are provided, preference is given to
// the command line writeConcern option. If neither is provided, the default 'majority' write concern is constructed.
func NewMongoWriteConcernOpts(writeConcern string, cs *connstring.ConnString) (wco *WriteConcernOptions, err error) {

	// Log whatever write concern was generated
	defer func() {
		if wco != nil {
			log.Logvf(log.Info, "using write concern: %v", wco)
		}
	}()

	// URI Connection String provided but no String provided case; constructWCOptionsFromConnString handles
	// default for ConnString without write concern
	if writeConcern == "" && cs != nil {
		return constructWCOptionsFromConnString(cs)
	}

	// String case; constructWCOptionsFromString handles default for empty string
	return constructWCOptionsFromString(writeConcern)
}

// constructWCOptionsFromConnString takes in a parsed connection string and
// extracts values from it. If the ConnString has no write concern value, it defaults
// to 'majority'.
func constructWCOptionsFromConnString(cs *connstring.ConnString) (*WriteConcernOptions, error) {
	opts := &WriteConcernOptions{}

	switch {
	case cs.WNumberSet:
		if cs.WNumber < 0 {
			return nil, fmt.Errorf("invalid 'w' argument: %v", cs.WNumber)
		}
		opts.WNumberSet = cs.WNumberSet
		opts.WNumber = cs.WNumber
	case cs.WString != "":
		opts.WString = cs.WString
	default:
		opts.WString = majString
	}

	opts.J = cs.J
	opts.WTimeout = cs.WTimeout

	return opts, nil
}

// constructWCOptionsFromString takes in a write concern and attempts to
// extract values from it. It returns an error if it is unable to parse the
// string or if a parsed write concern field value is invalid.
func constructWCOptionsFromString(writeConcern string) (*WriteConcernOptions, error) {

	// Default case
	if writeConcern == "" {
		return &WriteConcernOptions{WString: majString}, nil
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
	opts, err := parseModeString(writeConcern)
	if err == nil {
		return opts, nil
	}

	return nil, err
}

// parseJSONWriteConcern converts a JSON map representing a write concern object
func parseJSONWriteConcern(jsonWriteConcern map[string]interface{}) (*WriteConcernOptions, error) {
	var err error
	var opts *WriteConcernOptions

	// Construct new options from 'w', if it exists; otherwise default to 'majority'
	if wVal, ok := jsonWriteConcern[w]; ok {
		opts, err = parseWField(wVal)
		if err != nil {
			return nil, err
		}
	} else {
		opts = &WriteConcernOptions{WString: majString}
	}

	// Journal option
	if jVal, ok := jsonWriteConcern[j]; ok && util.IsTruthy(jVal) {
		opts.J = true
	}

	// Wtimeout option
	if wtimeout, ok := jsonWriteConcern[wTimeout]; ok {
		timeoutVal, err := util.ToInt(wtimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid '%v' argument: %v", wTimeout, wtimeout)
		}
		// Previous implementation assumed passed in string was milliseconds
		opts.WTimeout = time.Duration(timeoutVal) * time.Millisecond
	}

	return opts, nil
}

func parseWField(wValue interface{}) (*WriteConcernOptions, error) {
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

// Given an integer, returns a write concern options object or error
func parseModeNumber(wNumber int) (*WriteConcernOptions, error) {
	if wNumber < 0 {
		return nil, fmt.Errorf("invalid 'w' argument: %v", wNumber)
	}
	opts := &WriteConcernOptions{
		WNumberSet: true,
		WNumber:    wNumber,
	}
	return opts, nil
}

// Given a string, returns a write concern options object or error
func parseModeString(wString string) (*WriteConcernOptions, error) {
	// Default case
	if wString == "" {
		return &WriteConcernOptions{WString: majString}, nil
	}

	// Try parsing as number before treating as just a string
	if wNumber, err := strconv.Atoi(wString); err == nil {
		return parseModeNumber(wNumber)
	}

	return &WriteConcernOptions{WString: wString}, nil
}

// ToMongoWriteConcern translates a WriteConcernOptions with public fields we
// can examine into an opaque MongoDB Go driver write concern.
func (wco WriteConcernOptions) ToMongoWriteConcern() *writeconcern.WriteConcern {
	switch {
	case wco.WNumberSet:
		return writeconcern.New(writeconcern.W(wco.WNumber), writeconcern.J(wco.J), writeconcern.WTimeout(wco.WTimeout))
	case wco.WString == majString:
		return writeconcern.New(writeconcern.WMajority(), writeconcern.J(wco.J), writeconcern.WTimeout(wco.WTimeout))
	default:
		return writeconcern.New(writeconcern.WTagSet(wco.WString), writeconcern.J(wco.J), writeconcern.WTimeout(wco.WTimeout))
	}
}

func (wco WriteConcernOptions) String() string {
	var w string
	if wco.WNumberSet {
		w = fmt.Sprintf("%d", wco.WNumber)
	} else {
		w = wco.WString
	}
	return fmt.Sprintf("w='%v', j=%v, wTimeout=%v", w, wco.J, wco.WTimeout)
}
