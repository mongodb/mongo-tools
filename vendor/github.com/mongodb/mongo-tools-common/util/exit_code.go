// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"errors"
)

const (
	ExitFailure int = 1
	ExitSuccess int = 0
)

var (
	ErrTerminated = errors.New("received termination signal")
)

// SetupError is the error thrown by "New" functions used to convey what error occurred and the appropriate exit code.
type SetupError struct {
	Err error

	// An optional message to be logged before exiting
	Message string
}

// Error implements the error interface.
func (se SetupError) Error() string {
	return se.Err.Error()
}
