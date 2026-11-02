//go:build unit

package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuoteAndJoin(t *testing.T) {
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
		{
			"mongodb://user:pass@example.com/db?tlsCertificateKeyFilePassword=keySecretXYZ",
			"mongodb://[**REDACTED**]@example.com/db?tlsCertificateKeyFilePassword=[**REDACTED**]",
		},
		{
			"mongodb://example.com/db?tls=true&sslClientCertificateKeyPassword=keySecretXYZ",
			"mongodb://example.com/db?tls=true&sslClientCertificateKeyPassword=[**REDACTED**]",
		},
		{
			"mongodb://example.com/db?AuthMechanismProperties=AWS_SESSION_TOKEN:token",
			"mongodb://example.com/db?AuthMechanismProperties=[**REDACTED**]",
		},
		{
			"mongodb://example.com/db?appName=password",
			"mongodb://example.com/db?appName=password",
		},
	}

	for _, c := range cases {
		assert.Equal(t, c[1], SanitizeURI(c[0]), c[0])
	}
}

func TestSanitizeURIError(t *testing.T) {
	cases := []struct {
		description string
		err         error
		uri         string
		expected    string
	}{
		{
			"quoted query secret",
			errors.New(`invalid option value "key%ZZ": invalid URL escape "%ZZ"`),
			"mongodb://user:pass@example.com/db?tlsCertificateKeyFilePassword=key%ZZ",
			`invalid option value "[**REDACTED**]": invalid URL escape "%ZZ"`,
		},
		{
			"quoted authMechanismProperties secret",
			errors.New(`invalid option value "AWS_SESSION_TOKEN:tok%ZZ": invalid URL escape "%ZZ"`),
			"mongodb://user:pass@example.com/db?authMechanismProperties=AWS_SESSION_TOKEN:tok%ZZ",
			`invalid option value "[**REDACTED**]": invalid URL escape "%ZZ"`,
		},
		{
			"embedded URI",
			errors.New(`parse "mongodb://user:pass%ZZ@example.com/": invalid URL escape "%ZZ"`),
			"mongodb://user:pass%ZZ@example.com/",
			`parse "mongodb://[**REDACTED**]@example.com/": invalid URL escape "%ZZ"`,
		},
		{
			"no secret",
			errors.New("some other error"),
			"mongodb://example.com/",
			"some other error",
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			assert.Equal(t, c.expected, SanitizeURIError(c.err, c.uri).Error())
		})
	}

	assert.NoError(t, SanitizeURIError(nil, "mongodb://example.com/"))
}
