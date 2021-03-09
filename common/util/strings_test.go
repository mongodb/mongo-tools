package util

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
)

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
