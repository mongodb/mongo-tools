// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package failpoint implements triggers for custom debugging behavior.
package failpoint

import "sync"

// pauseSignal coordinates a single pause: reached is closed by wait to
// signal that the paused code has arrived at its pause point, and signal is
// closed by signal to release it. mu guards the two closed flags so that
// closing either channel more than once is a safe no-op rather than a panic.
type pauseSignal struct {
	mu            sync.Mutex
	reachedCh     chan struct{}
	reachedClosed bool
	signalCh      chan struct{}
	signalClosed  bool
}

func newPauseSignal() *pauseSignal {
	return &pauseSignal{
		reachedCh: make(chan struct{}),
		signalCh:  make(chan struct{}),
	}
}

// wait closes reachedCh (so a concurrent call to reached returns) and then
// blocks until signal is called.
func (p *pauseSignal) wait() {
	p.mu.Lock()
	if !p.reachedClosed {
		close(p.reachedCh)
		p.reachedClosed = true
	}
	p.mu.Unlock()

	<-p.signalCh
}

// reached blocks until wait has been called.
func (p *pauseSignal) reached() {
	<-p.reachedCh
}

// signal releases a call blocked in wait. Safe to call more than once.
func (p *pauseSignal) signal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.signalClosed {
		close(p.signalCh)
		p.signalClosed = true
	}
}

// Failpoint represents one named failpoint's runtime state — for failpoints
// that need to pause code until externally resumed, the signaling to
// coordinate that pause.
type Failpoint struct {
	pause *pauseSignal
}

func newFailpoint() *Failpoint {
	return &Failpoint{
		pause: newPauseSignal(),
	}
}

// Wait signals that the caller has reached this pause point (so a concurrent
// call to Reached returns) and then blocks until Signal is called.
func (fp *Failpoint) Wait() { fp.pause.wait() }

// Reached blocks until Wait has been called.
func (fp *Failpoint) Reached() { fp.pause.reached() }

// Signal releases a call blocked in Wait. Safe to call more than once.
func (fp *Failpoint) Signal() { fp.pause.signal() }

// Manager tracks which failpoints are currently enabled.
type Manager interface {
	// Parse enables a comma-separated list of failpoint names, as passed to
	// --failpoints. It returns an error if arg contains an unknown name.
	Parse(arg string) error
	// Reset disables every failpoint.
	Reset()
	// Get returns the Failpoint registered under name, and true, if it is
	// currently enabled.
	Get(name Name) (*Failpoint, bool)
}

// DefaultManager is set by manager.go's or noop_manager.go's init function,
// depending on whether this binary was built with the failpoints build tag.
// Callers use it directly, e.g. failpoint.DefaultManager.Get(failpoint.Foo).
var DefaultManager Manager
