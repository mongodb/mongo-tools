// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

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

var uriRedactionRE = regexp.MustCompile(`^([^:]+)://[^/?]*@`)

func SanitizeURI(u string) string {
	return uriRedactionRE.ReplaceAllString(u, "$1://[**REDACTED**]@")
}
