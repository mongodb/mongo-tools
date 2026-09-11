// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package intents

import (
	"sort"
	"sync"
)

type PriorityType int

const (
	Legacy PriorityType = iota
	LongestTaskFirst
)

// IntentPrioritizer encapsulates the logic of scheduling intents
// for restoration. It can know about which intents are in the
// process of being restored through the "Finish" hook.
//
// Oplog entries and auth entries are not handled by the prioritizer,
// as these are special cases handled by the regular mongorestore code.
type IntentPrioritizer interface {
	Get() *Intent
	Finish(*Intent)
}

//===== Legacy =====

// legacyPrioritizer processes the intents in the order they were read off the
// file system, keeping with legacy mongorestore behavior.
type legacyPrioritizer struct {
	sync.Mutex
	queue []*Intent
}

func newLegacyPrioritizer(intentList []*Intent) *legacyPrioritizer {
	return &legacyPrioritizer{queue: intentList}
}

func (legacy *legacyPrioritizer) Get() *Intent {
	legacy.Lock()
	defer legacy.Unlock()

	if len(legacy.queue) == 0 {
		return nil
	}

	var intent *Intent
	intent, legacy.queue = legacy.queue[0], legacy.queue[1:]
	return intent
}

func (legacy *legacyPrioritizer) Finish(*Intent) {
	// no-op
	return
}

//===== Longest Task First =====

// longestTaskFirstPrioritizer returns intents in the order of largest -> smallest,
// with views at the front of the list, which is better at minimizing total
// runtime in parallel environments than other simple orderings.
type longestTaskFirstPrioritizer struct {
	sync.Mutex
	queue []*Intent
}

// newLongestTaskFirstPrioritizer returns an initialized LTP prioritizer.
func newLongestTaskFirstPrioritizer(intents []*Intent) *longestTaskFirstPrioritizer {
	sort.Sort(BySizeAndView(intents))
	return &longestTaskFirstPrioritizer{
		queue: intents,
	}
}

func (ltf *longestTaskFirstPrioritizer) Get() *Intent {
	ltf.Lock()
	defer ltf.Unlock()

	if len(ltf.queue) == 0 {
		return nil
	}

	var intent *Intent
	intent, ltf.queue = ltf.queue[0], ltf.queue[1:]
	return intent
}

func (ltf *longestTaskFirstPrioritizer) Finish(*Intent) {
	// no-op
	return
}

// BySizeAndView attaches the methods for sort.Interface for sorting intents
// from largest to smallest size, taking into account if it's a view or not.
type BySizeAndView []*Intent

func (s BySizeAndView) Len() int      { return len(s) }
func (s BySizeAndView) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s BySizeAndView) Less(i, j int) bool {
	if s[i].IsView() && !s[j].IsView() {
		return true
	}
	if !s[i].IsView() && s[j].IsView() {
		return false
	}
	return s[i].Size > s[j].Size
}
