// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"github.com/mongodb/mongo-tools-common/connstring"
	"github.com/mongodb/mongo-tools-common/json"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"gopkg.in/mgo.v2"

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

type WriteConcernOptions struct {
	W         int
	J         bool
	WMajority bool
	WTagSet   string
	WTimeout  time.Duration
}

// constructWCObject takes in a write concern and attempts to construct an
// mgo.Safe object from it. It returns an error if it is unable to parse the
// string or if a parsed write concern field value is invalid.
func constructWCObject(writeConcern string) (sessionSafety *mgo.Safe, err error) {
	sessionSafety = &mgo.Safe{}
	defer func() {
		// If the user passes a w value of 0, we set the session to use the
		// unacknowledged write concern but only if journal commit acknowledgment,
		// is not required. If commit acknowledgment is required, it prevails,
		// and the server will require that mongod acknowledge the write operation
		if sessionSafety.WMode == "" && sessionSafety.W == 0 && !sessionSafety.J {
			sessionSafety = nil
		}
	}()
	jsonWriteConcern := map[string]interface{}{}

	if err = json.Unmarshal([]byte(writeConcern), &jsonWriteConcern); err != nil {
		// if the writeConcern string can not be unmarshaled into JSON, this
		// allows a default to the old behavior wherein the entire argument
		// passed in is assigned to the 'w' field - thus allowing users pass
		// a write concern that looks like: "majority", 0, "4", etc.
		wValue, err := strconv.Atoi(writeConcern)
		if err != nil {
			sessionSafety.WMode = writeConcern
		} else {
			sessionSafety.W = wValue
			if wValue < 0 {
				return sessionSafety, fmt.Errorf("invalid '%v' argument: %v", w, wValue)
			}
		}
		return sessionSafety, nil
	}

	if jVal, ok := jsonWriteConcern[j]; ok && util.IsTruthy(jVal) {
		sessionSafety.J = true
	}

	if fsyncVal, ok := jsonWriteConcern[fSync]; ok && util.IsTruthy(fsyncVal) {
		sessionSafety.FSync = true
	}

	if wtimeout, ok := jsonWriteConcern[wTimeout]; ok {
		wtimeoutValue, err := util.ToInt(wtimeout)
		if err != nil {
			return sessionSafety, fmt.Errorf("invalid '%v' argument: %v", wTimeout, wtimeout)
		}
		sessionSafety.WTimeout = wtimeoutValue
	}

	if wInterface, ok := jsonWriteConcern[w]; ok {
		wValue, err := util.ToInt(wInterface)
		if err != nil {
			// if the argument is neither a string nor int, error out
			wStrVal, ok := wInterface.(string)
			if !ok {
				return sessionSafety, fmt.Errorf("invalid '%v' argument: %v", w, wInterface)
			}
			sessionSafety.WMode = wStrVal
		} else {
			sessionSafety.W = wValue
			if wValue < 0 {
				return sessionSafety, fmt.Errorf("invalid '%v' argument: %v", w, wValue)
			}
		}
	}
	return sessionSafety, nil
}

// constructSafetyFromConnString takes in a parsed connection string and attempts
// to construct an mgo.Safe object from it. It returns an error if it is unable
// to parse the write concern value.
func constructSafetyFromConnString(cs *connstring.ConnString) (*mgo.Safe, error) {
	safe := &mgo.Safe{}

	wValue, err := strconv.Atoi(cs.W)
	if err != nil {
		safe.WMode = cs.W
	} else {
		safe.W = wValue
		if wValue < 0 {
			return nil, fmt.Errorf("invalid '%v' argument: %v", w, wValue)
		}
	}

	safe.WTimeout = int(cs.WTimeout / time.Second)
	safe.FSync = cs.FSync
	safe.J = cs.Journal

	if safe.WMode == "" && safe.W == 0 && !safe.J {
		return nil, nil
	}

	return safe, nil
}

// BuildWriteConcern takes a string and a NodeType indicating the type of node the write concern
// is intended to be used against, and converts the write concern string argument into an
// mgo.Safe object that's usable on sessions for that node type.
func BuildWriteConcern(writeConcern string, nodeType NodeType, cs *connstring.ConnString) (*mgo.Safe, error) {
	var sessionSafety *mgo.Safe
	var err error

	if cs != nil && writeConcern != "" {
		return nil, fmt.Errorf("cannot specify writeConcern string and connectionString object")
	}

	if cs != nil {
		if cs.W == "" {
			cs.W = majString
		}
		sessionSafety, err = constructSafetyFromConnString(cs)
		if err != nil {
			return nil, err
		}
	} else {
		if writeConcern == "" {
			writeConcern = majString
		}
		sessionSafety, err = constructWCObject(writeConcern)
		if err != nil {
			return nil, err
		}
	}

	if sessionSafety == nil {
		log.Logvf(log.DebugLow, "using unacknowledged write concern")
		return nil, nil
	}

	// for standalone mongods, set the default write concern to 1
	if nodeType == Standalone {
		log.Logvf(log.DebugLow, "standalone server: setting write concern %v to 1", w)
		sessionSafety.W = 1
		sessionSafety.WMode = ""
	}

	var writeConcernStr interface{}

	if sessionSafety.WMode != "" {
		writeConcernStr = sessionSafety.WMode
	} else {
		writeConcernStr = sessionSafety.W
	}
	log.Logvf(log.Info, "using write concern: %v='%v', %v=%v, %v=%v, %v=%v",
		w, writeConcernStr,
		j, sessionSafety.J,
		fSync, sessionSafety.FSync,
		wTimeout, sessionSafety.WTimeout,
	)
	return sessionSafety, nil
}

// Takes in a WriteConcernOptions and a string representing the desired write concern mode.
// Sets the correct values in the passed in options object, and returns an error if neccesary.
func parseModeString(opts *WriteConcernOptions, writeConcern string) error {
	// if the writeConcern string can not be unmarshaled into JSON, this
	// allows a default to the old behavior wherein the entire argument
	// passed in is assigned to the 'w' field - thus allowing users pass
	// a write concern that looks like: "majority", 0, "4", etc.
	wValue, err := strconv.Atoi(writeConcern)
	if err != nil {
		if writeConcern == majString {
			opts.WMajority = true
		} else {
			opts.WTagSet = writeConcern
		}
	} else {
		opts.W = wValue
		if wValue < 0 {
			return fmt.Errorf("invalid '%v' argument: %v", w, wValue)
		}
	}
	return nil
}

// If all of the options in the passed in WriteConcernOptions are unset, return true.
func useUnacknowledgedWriteConcern(opts *WriteConcernOptions) bool {
	if opts.W == 0 && !opts.J && opts.WMajority == false && opts.WTagSet == "" {
		return true
	}
	return false

}

// constructWCOptionsFromString takes in a write concern and attempts to extract values
// from it. It returns an error if it is unable to parse the
// string or if a parsed write concern field value is invalid.
// Returns a WriteConcernOptions object.
func constructWCOptionsFromString(writeConcern string) (*WriteConcernOptions, error) {
	opts := &WriteConcernOptions{}
	var err error
	defer func() {
		// If the user passes a w value of 0, we set the session to use the
		// unacknowledged write concern but only if journal commit acknowledgment,
		// is not required. If commit acknowledgment is required, it prevails,
		// and the server will require that mongod acknowledge the write operation
		if useUnacknowledgedWriteConcern(opts) {
			opts = nil
		}
	}()

	jsonWriteConcern := map[string]interface{}{}

	if err = json.Unmarshal([]byte(writeConcern), &jsonWriteConcern); err != nil {
		err = parseModeString(opts, writeConcern)
		if err != nil {
			return opts, err
		}
	}

	if jVal, ok := jsonWriteConcern[j]; ok && util.IsTruthy(jVal) {
		opts.J = true
	}

	if wtimeout, ok := jsonWriteConcern[wTimeout]; ok {
		timeoutVal, err := util.ToInt(wtimeout)
		if err != nil {
			return opts, fmt.Errorf("invalid '%v' argument: %v", wTimeout, wtimeout)
		}
		// Previous implementation assumed passed in string was milliseconds
		opts.WTimeout = time.Duration(timeoutVal) * time.Millisecond
	}

	if wInterface, ok := jsonWriteConcern[w]; ok {
		wValue, err := util.ToInt(wInterface)
		if err != nil {
			// if the argument is neither a string nor int, error out
			wStrVal, ok := wInterface.(string)
			if !ok {
				return opts, fmt.Errorf("invalid '%v' argument: %v", w, wInterface)
			}
			err = parseModeString(opts, wStrVal)
			if err != nil {
				return opts, err
			}
		} else {
			opts.W = wValue
			if wValue < 0 {
				return opts, fmt.Errorf("invalid '%v' argument: %v", w, wValue)
			}
		}
	}
	return opts, nil
}

// constructWCOptionsFromConnString takes in a parsed connection string and attempts
// to extract values from it. Returns a WriteConcernsOptions.
// It returns an error if it is unable to parse the write concern value.
func constructWCOptionsFromConnString(cs *connstring.ConnString) (*WriteConcernOptions, error) {
	opts := &WriteConcernOptions{}

	err := parseModeString(opts, cs.W)
	if err != nil {
		return opts, err
	}
	opts.J = cs.Journal
	opts.WTimeout = cs.WTimeout

	if useUnacknowledgedWriteConcern(opts) {
		return nil, nil
	}

	return opts, nil
}

// BuildWriteConcern takes a string and converts the write concern string argument into a
// WriteConcern options object. Only one of writeConcern and cs can be specified.
func BuildMongoWriteConcernOpts(writeConcern string, cs *connstring.ConnString) (*WriteConcernOptions, error) {
	opts := &WriteConcernOptions{}
	var err error

	if cs != nil && writeConcern != "" {
		return nil, fmt.Errorf("cannot specify writeConcern string and connectionString object")
	}

	if cs != nil {
		if cs.W == "" {
			cs.W = majString
		}
		opts, err = constructWCOptionsFromConnString(cs)
		if err != nil {
			return nil, err
		}
	} else {
		if writeConcern == "" {
			writeConcern = majString
		}
		opts, err = constructWCOptionsFromString(writeConcern)
		if err != nil {
			return nil, err
		}
	}

	if opts == nil || useUnacknowledgedWriteConcern(opts) {
		log.Logvf(log.DebugLow, "using unacknowledged write concern")
		return nil, nil
	}

	log.Logvf(log.Info, "using write concern: %v='%v', %v=%v, %v=%v, %v=%v",
		w, opts.W,
		j, opts.J,
		wTimeout, opts.WTimeout,
		majString, opts.WMajority,
	)

	return opts, nil
}

// A wrapper for BuildMongoWriteConcern. Tests will call BuildMongoWriteConcern because they can read the options. Real users will call this.
// Temporary until Go Driver adds accessors.
func PackageWriteConcernOptsToObject(writeConcern string, cs *connstring.ConnString) (*writeconcern.WriteConcern, error) {
	opts, err := BuildMongoWriteConcernOpts(writeConcern, cs)
	if opts == nil {
		return nil, err
	}
	if opts.WMajority {
		return writeconcern.New(writeconcern.WMajority(), writeconcern.J(opts.J), writeconcern.WTimeout(opts.WTimeout)), err
	}
	if opts.WTagSet != "" {
		return writeconcern.New(writeconcern.J(opts.J), writeconcern.WTimeout(opts.WTimeout), writeconcern.WTagSet(opts.WTagSet)), err
	}
	return writeconcern.New(writeconcern.W(opts.W), writeconcern.J(opts.J), writeconcern.WTimeout(opts.WTimeout)), err

}
