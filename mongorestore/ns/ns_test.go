// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package ns

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// bump up the verbosity to make checking debug log output possible
	log.SetVerbosity(&options.Verbosity{
		VLevel: 4,
	})
}

func TestEscape(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		in       string
		expected string
	}{
		{"(blah)", "(blah)"},
		{"", ""},
		{`bl*h*\\`, `bl\*h\*\\\\`},
		{"blah**", `blah\*\*`},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.expected, Escape(tc.in), "should escape %q", tc.in)
	}
}

func TestUnescape(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		in       string
		expected string
	}{
		{"(blah)", "(blah)"},
		{"", ""},
		{`bl\*h\*\\\\`, `bl*h*\\`},
		{`blah\*\*`, "blah**"},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.expected, Unescape(tc.in), "should unescape %q", tc.in)
	}
}

func TestReplacer(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("with replacements", func(t *testing.T) {
		cases := []struct {
			name     string
			from     []string
			to       []string
			expected map[string]string
		}{
			{
				name: `'$db$.user$$' -> 'test.user$$_$db$', 'pr\*d\.*' -> 'st\*g\\ing.*'`,
				from: []string{"$db$.user$$", `pr\*d\\.*`},
				to:   []string{"test.user$$_$db$", `st\*g\\ing.*`},
				expected: map[string]string{
					"stuff.user":                  "test.user_stuff",
					"stuff.users":                 "test.users_stuff",
					`pr*d\.users`:                 `st*g\ing.users`,
					`pr*d\.turbo.encabulators`:    `st*g\ing.turbo.encabulators`,
					`st*g\ing.turbo.encabulators`: `st*g\ing.turbo.encabulators`,
				},
			},
			{
				name: "'$:)*$.us(?:2)er$?$' -> 'test.us(?:2)er$?$_$:)*$'",
				from: []string{"$:)*$.us(?:2)er$?$"},
				to:   []string{"test.us(?:2)er$?$_$:)*$"},
				expected: map[string]string{
					"stuff.us(?:2)er":  "test.us(?:2)er_stuff",
					"stuff.us(?:2)ers": "test.us(?:2)ers_stuff",
				},
			},
			{
				name: "'*.*' -> '*_test.*'",
				from: []string{"*.*"},
				to:   []string{"*_test.*"},
				expected: map[string]string{
					"stuff.user":              "stuff_test.user",
					"stuff.users":             "stuff_test.users",
					"prod.turbo.encabulators": "prod_test.turbo.encabulators",
				},
			},
			{
				name: "special characters",
				from: []string{`restaurants.cafés`, `ÿœz.tāx`, `normal.characters`},
				to:   []string{`ÿœp.tāx`, `yes.tax`, `special.charâctęrs`},
				expected: map[string]string{
					"restaurants.cafés": "ÿœp.tāx",
					"ÿœz.tāx":           "yes.tax",
					"normal.characters": "special.charâctęrs",
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r, err := NewRenamer(tc.from, tc.to)
				require.NoError(t, err, "should build a renamer")
				require.NotNil(t, r, "should return a renamer")
				for in, expected := range tc.expected {
					assert.Equal(t, expected, r.Get(in), "should rename %q", in)
				}
			})
		}
	})

	t.Run("with invalid replacements", func(t *testing.T) {
		cases := []struct{ from, to string }{
			{"$db$.user$db$", "test.user-$db$"},
			{"$db$.us$er$table$", "test.user$table$_$db$"},
		}
		for _, tc := range cases {
			_, err := NewRenamer([]string{tc.from}, []string{tc.to})
			assert.Error(t, err, "should reject replacement %q -> %q", tc.from, tc.to)
		}
	})
}

func TestMatcher(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("with matcher", func(t *testing.T) {
		cases := []struct {
			name     string
			patterns []string
			expected map[string]bool
		}{
			{
				name:     `'*.user*', 'pr\*d\.*'`,
				patterns: []string{`*.user*`, `pr\*d\.*`},
				expected: map[string]bool{
					"stuff.user":                 true,
					"stuff.users":                true,
					"pr*d.users":                 true,
					"pr*d.magic":                 true,
					`pr*d\.magic`:                false,
					"prod.magic":                 false,
					"pr*d.turbo.encabulators":    true,
					"st*ging.turbo.encabulators": false,
				},
			},
			{
				name:     "'*.*'",
				patterns: []string{"*.*"},
				expected: map[string]bool{
					"stuff":                   false,
					"stuff.user":              true,
					"stuff.users":             true,
					"prod.turbo.encabulators": true,
				},
			},
			{
				name:     "special characters",
				patterns: []string{`restaurants.cafés`, `ÿœp.tāx`},
				expected: map[string]bool{
					"restaurants.cafés": true,
					"ÿœp.tāx":           true,
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m, err := NewMatcher(tc.patterns)
				require.NoError(t, err, "should build a matcher")
				require.NotNil(t, m, "should return a matcher")
				for in, expected := range tc.expected {
					assert.Equal(
						t,
						expected,
						m.Has(in),
						"should report %q as matching=%v",
						in,
						expected,
					)
				}
			})
		}
	})

	t.Run("with invalid matcher", func(t *testing.T) {
		cases := []string{"$.user$", "*.user$"}
		for _, pattern := range cases {
			_, err := NewMatcher([]string{pattern})
			assert.Error(t, err, "should reject pattern %q", pattern)
		}
	})
}
