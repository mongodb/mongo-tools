// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build !failpoints

package failpoint

// noopManager is the Manager implementation used when this binary is built
// without the failpoints build tag: every failpoint is always disabled.
type noopManager struct{}

func newNoopManager() *noopManager {
	return &noopManager{}
}

func (*noopManager) Parse(arg string) error {
	_, err := parseNames(arg)
	return err
}

func (*noopManager) Reset() {}

func (*noopManager) Get(Name) (*Failpoint, bool) {
	return nil, false
}

func init() {
	DefaultManager = newNoopManager()
}
