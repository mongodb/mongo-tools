// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/lo"
)

// RedactedValue replaces credentials in sanitized strings.
const RedactedValue = "[**REDACTED**]"

// Pluralize takes an amount and two strings denoting the singular
// and plural noun the amount represents. If the amount is singular,
// the singular form is returned; otherwise plural is returned. E.g.
//
//	Pluralize(X, "mouse", "mice") -> 0 mice, 1 mouse, 2 mice, ...
func Pluralize(amount int, singular, plural string) string {
	if amount == 1 {
		return singular
	}
	return plural
}

// QuoteAndJoin quotes each string in ss using Go's %#q format, then joins
// them with sep. This makes special characters in each element visible in log
// output.
func QuoteAndJoin(ss []string, sep string) string {
	return strings.Join(lo.Map(ss, func(s string, _ int) string {
		return fmt.Sprintf("%#q", s)
	}), sep)
}

var (
	uriRedactionRE = regexp.MustCompile(`^([^:]+)://[^/?]*@`)

	// Connection strings may carry credentials in query parameters, e.g.
	// tlsCertificateKeyFilePassword or an AWS_SESSION_TOKEN inside
	// authMechanismProperties. Match these directly instead of parsing the URL
	// because SanitizeURI is also used on strings that do not parse.
	uriSecretQueryRE = regexp.MustCompile(
		`(?i)([?&;](?:[^=&#;]*(?:password|secret|token|credential)[^=&#;]*|authmechanismproperties)=)[^&#;]*`,
	)

	// uriSecretValueRE captures the raw value of a secret query parameter so it
	// can be redacted from an error message that quotes just the value.
	uriSecretValueRE = regexp.MustCompile(
		`(?i)[?&;](?:[^=&#;]*(?:password|secret|token|credential)[^=&#;]*|authmechanismproperties)=([^&#;]*)`,
	)
)

// SanitizeURI returns u with credentials replaced by RedactedValue. It handles
// both userinfo credentials and credentials in query parameters.
func SanitizeURI(u string) string {
	return sanitizeURI(u, u)
}

// SanitizeURIError returns err with any secrets taken from uri removed from its
// message. Use this instead of SanitizeURI when the text is an error message
// that may quote individual values from uri rather than the URI itself. For
// example, a connection string parsing error can quote the raw value of a
// malformed credential.
func SanitizeURIError(err error, uri string) error {
	if err == nil {
		return nil
	}

	return errors.New(sanitizeURI(err.Error(), uri))
}

func sanitizeURI(text, uri string) string {
	// Replace an embedded URI wholesale first, because error messages such as
	// url.Parse errors quote the whole URI rather than just a secret value.
	if uri != "" {
		text = strings.ReplaceAll(text, uri, redactURIParts(uri))
	}
	text = redactURIParts(text)

	// Replace longer values first so that a value that contains another secret
	// is redacted completely.
	for _, secret := range uriSecretValues(uri) {
		text = strings.ReplaceAll(text, secret, RedactedValue)
	}

	return text
}

func redactURIParts(uri string) string {
	uri = uriRedactionRE.ReplaceAllString(uri, "$1://"+RedactedValue+"@")
	return uriSecretQueryRE.ReplaceAllString(uri, "$1"+RedactedValue)
}

func uriSecretValues(uri string) []string {
	var secrets []string
	for _, match := range uriSecretValueRE.FindAllStringSubmatch(uri, -1) {
		if len(match[1]) > 0 {
			secrets = append(secrets, match[1])
		}
	}

	slices.SortFunc(secrets, func(a, b string) int {
		return len(b) - len(a)
	})
	return secrets
}
