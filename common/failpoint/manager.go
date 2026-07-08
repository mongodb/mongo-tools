// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build failpoints

package failpoint

import "sync"

// manager is the real Manager implementation, used when this binary is
// built with the failpoints build tag.
type manager struct {
	mu  sync.Mutex
	fps map[Name]*Failpoint
}

func newManager() *manager {
	return &manager{fps: make(map[Name]*Failpoint)}
}

func (m *manager) Parse(arg string) error {
	names, err := parseNames(arg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, name := range names {
		m.fps[name] = newFailpoint()
	}
	return nil
}

func (m *manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fps = make(map[Name]*Failpoint)
}

func (m *manager) Get(name Name) (*Failpoint, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fp, ok := m.fps[name]
	return fp, ok
}

func init() {
	DefaultManager = newManager()
}
