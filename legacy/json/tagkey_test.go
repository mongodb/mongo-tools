// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
//
// Based on github.com/golang/go by The Go Authors
// See THIRD-PARTY-NOTICES for original license terms.

package json

import (
	"github.com/mongodb/mongo-tools/legacy/testtype"
	"testing"
)

type basicLatin2xTag struct {
	V string `json:"$%-/"`
}

type basicLatin3xTag struct {
	V string `json:"0123456789"`
}

type basicLatin4xTag struct {
	V string `json:"ABCDEFGHIJKLMO"`
}

type basicLatin5xTag struct {
	V string `json:"PQRSTUVWXYZ_"`
}

type basicLatin6xTag struct {
	V string `json:"abcdefghijklmno"`
}

type basicLatin7xTag struct {
	V string `json:"pqrstuvwxyz"`
}

type miscPlaneTag struct {
	V string `json:"色は匂へど"`
}

type percentSlashTag struct {
	V string `json:"text/html%"` // http://golang.org/issue/2718
}

type punctuationTag struct {
	V string `json:"!#$%&()*+-./:<=>?@[]^_{|}~"` // http://golang.org/issue/3546
}

type emptyTag struct {
	W string
}

type misnamedTag struct {
	X string `jsom:"Misnamed"`
}

type badCodeTag struct {
	Z string `json:" !\"#&'()*+,."`
}

type spaceTag struct {
	Q string `json:"With space"`
}

type unicodeTag struct {
	W string `json:"Ελλάδα"`
}

var structTagObjectKeyTests = []struct {
	raw   interface{}
	value string
	key   string
}{
	{basicLatin2xTag{"2x"}, "2x", "$%-/"},
	{basicLatin3xTag{"3x"}, "3x", "0123456789"},
	{basicLatin4xTag{"4x"}, "4x", "ABCDEFGHIJKLMO"},
	{basicLatin5xTag{"5x"}, "5x", "PQRSTUVWXYZ_"},
	{basicLatin6xTag{"6x"}, "6x", "abcdefghijklmno"},
	{basicLatin7xTag{"7x"}, "7x", "pqrstuvwxyz"},
	{miscPlaneTag{"いろはにほへと"}, "いろはにほへと", "色は匂へど"},
	{emptyTag{"Pour Moi"}, "Pour Moi", "W"},
	{misnamedTag{"Animal Kingdom"}, "Animal Kingdom", "X"},
	{badCodeTag{"Reliable Man"}, "Reliable Man", "Z"},
	{percentSlashTag{"brut"}, "brut", "text/html%"},
	{punctuationTag{"Union Rags"}, "Union Rags", "!#$%&()*+-./:<=>?@[]^_{|}~"},
	{spaceTag{"Perreddu"}, "Perreddu", "With space"},
	{unicodeTag{"Loukanikos"}, "Loukanikos", "Ελλάδα"},
}

func TestStructTagObjectKey(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)
	for _, tt := range structTagObjectKeyTests {
		b, err := Marshal(tt.raw)
		if err != nil {
			t.Fatalf("Marshal(%#q) failed: %v", tt.raw, err)
		}
		var f interface{}
		err = Unmarshal(b, &f)
		if err != nil {
			t.Fatalf("Unmarshal(%#q) failed: %v", b, err)
		}
		for i, v := range f.(map[string]interface{}) {
			switch i {
			case tt.key:
				if s, ok := v.(string); !ok || s != tt.value {
					t.Fatalf("Unexpected value: %#q, want %v", s, tt.value)
				}
			default:
				t.Fatalf("Unexpected key: %#q, from %#q", i, b)
			}
		}
	}
}
