package util

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
)

func TestQuoteAndJoin(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := []struct {
		input   []string
		sep     string
		expect  string
		message string
	}{
		{
			input:   []string{"foo", "bar", "baz"},
			sep:     ", ",
			expect:  "`foo`, `bar`, `baz`",
			message: "plain strings are backtick-quoted",
		},
		{
			input:   []string{"has space", "has\nnewline"},
			sep:     ", ",
			expect:  "`has space`, \"has\\nnewline\"",
			message: "strings with spaces use backticks; newlines force double-quoting with escaping",
		},
		{
			input:   []string{"weird\x00name"},
			sep:     ",",
			expect:  `"weird\x00name"`,
			message: "null bytes are escaped with double-quoting",
		},
		{
			input:   []string{"has`backtick"},
			sep:     ",",
			expect:  "\"has`backtick\"",
			message: "strings containing backticks are double-quoted",
		},
		{
			input:   []string{},
			sep:     ", ",
			expect:  "",
			message: "empty slice produces empty string",
		},
		{
			input:   []string{"only"},
			sep:     ", ",
			expect:  "`only`",
			message: "single element has no separator",
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.expect, QuoteAndJoin(c.input, c.sep), c.message)
	}
}

func TestSanitizeURI(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	cases := [][]string{
		{"mongodb://example.com/", "mongodb://example.com/"},
		{"mongodb://example.com/?appName=foo:@bar", "mongodb://example.com/?appName=foo:@bar"},
		{"mongodb://example.com?appName=foo:@bar", "mongodb://example.com?appName=foo:@bar"},
		{"mongodb://@example.com/", "mongodb://[**REDACTED**]@example.com/"},
		{"mongodb://:@example.com/", "mongodb://[**REDACTED**]@example.com/"},
		{"mongodb://user@example.com/", "mongodb://[**REDACTED**]@example.com/"},
		{"mongodb://user:@example.com/", "mongodb://[**REDACTED**]@example.com/"},
		{"mongodb://:pass@example.com/", "mongodb://[**REDACTED**]@example.com/"},
		{"mongodb://user:pass@example.com/", "mongodb://[**REDACTED**]@example.com/"},
		{"mongodb+srv://example.com/", "mongodb+srv://example.com/"},
		{"mongodb+srv://@example.com/", "mongodb+srv://[**REDACTED**]@example.com/"},
		{"mongodb+srv://:@example.com/", "mongodb+srv://[**REDACTED**]@example.com/"},
		{"mongodb+srv://user@example.com/", "mongodb+srv://[**REDACTED**]@example.com/"},
		{"mongodb+srv://user:@example.com/", "mongodb+srv://[**REDACTED**]@example.com/"},
		{"mongodb+srv://:pass@example.com/", "mongodb+srv://[**REDACTED**]@example.com/"},
		{"mongodb+srv://user:pass@example.com/", "mongodb+srv://[**REDACTED**]@example.com/"},
	}

	for _, c := range cases {
		got := SanitizeURI(c[0])
		if got != c[1] {
			t.Errorf("For %s: got: %s; wanted: %s", c[0], got, c[1])
		}
	}
}
