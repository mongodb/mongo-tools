// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build !failpoints
// +build !failpoints

package failpoint

func ParseFailpoints(_ string) {
}

func Reset() {
	return
}

func Get(fp string) (string, bool) {
	return "", false
}

func Enabled(fp string) bool {
	return false
}
