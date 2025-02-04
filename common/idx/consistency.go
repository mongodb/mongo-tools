package idx

import (
	"fmt"
	"strings"
)

var fieldTypeRequiredOpts = []struct {
	fieldType string
	option    string
}{
	{"2dsphere", "2dsphereIndexVersion"},
	{"text", "textIndexVersion"},
}

func (idx IndexDocument) FindInconsistency() error {
	// []any is for easier inclusion into fmt.Errorf below.
	var errors []any

	for _, keySpec := range idx.Key {
		for _, ftro := range fieldTypeRequiredOpts {
			if keySpec.Value == ftro.fieldType {
				if _, hasOpt := idx.Options[ftro.option]; !hasOpt {
					errors = append(errors, fmt.Errorf("index %#q includes a %#q field (%#q) but lacks a %#q", idx.Options["name"], ftro.fieldType, keySpec.Key, ftro.option))
				}
			}
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return fmt.Errorf(
		strings.Join(repeat(len(errors), "%w"), "; "),
		errors...,
	)
}

func repeat[T any](count int, prototype T) []T {
	retval := make([]T, count)
	for i := range count {
		retval[i] = prototype
	}

	return retval
}
