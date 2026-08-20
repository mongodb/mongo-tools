// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"fmt"
	"reflect"
)

// Numeric Conversion Tools

type converterFunc func(any) (any, error)

// this helper makes it simple to generate new numeric converters,
// be sure to assign them on a package level instead of dynamically
// within a function to avoid low performance.
func newNumberConverter(targetType reflect.Type) converterFunc {
	return func(number any) (any, error) {
		// to avoid panics on nil values
		if number == nil {
			return nil, fmt.Errorf("cannot convert nil value")
		}
		v := reflect.ValueOf(number)
		if !v.Type().ConvertibleTo(targetType) {
			return nil, fmt.Errorf("cannot convert %v to %v", v.Type(), targetType)
		}
		converted := v.Convert(targetType)
		return converted.Interface(), nil
	}
}

// making this package level so it is only evaluated once.
var uint32Converter = newNumberConverter(reflect.TypeFor[uint32]())

// ToUInt32 is a function for converting any numeric type
// into a uint32. This can easily result in a loss of information
// due to truncation, so be careful.
func ToUInt32(number any) (uint32, error) {
	asInterface, err := uint32Converter(number)
	if err != nil {
		return 0, err
	}

	return convert[uint32](asInterface)
}

var intConverter = newNumberConverter(reflect.TypeFor[int]())

// ToInt is a function for converting any numeric type
// into an int. This can easily result in a loss of information
// due to truncation of floats.
func ToInt(number any) (int, error) {
	asInterface, err := intConverter(number)
	if err != nil {
		return 0, err
	}

	return convert[int](asInterface)
}

var float64Converter = newNumberConverter(reflect.TypeFor[float64]())

// ToFloat64 is a function for converting any numeric type
// into a float64.
func ToFloat64(number any) (float64, error) {
	asInterface, err := float64Converter(number)
	if err != nil {
		return 0, err
	}

	return convert[float64](asInterface)
}

func convert[T any](val any) (T, error) {
	converted, ok := val.(T)
	if !ok {
		return *new(T), fmt.Errorf("Expected %+v (%T) to be float64", val, val)
	}

	return converted, nil
}
