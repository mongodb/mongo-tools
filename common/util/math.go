package util

import (
	"fmt"
	"reflect"
)

// Return the max of two ints
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Numeric Conversion Tools

type converterFunc func(interface{}) (interface{}, error)

// this helper makes it simple to generate new numeric converters,
// be sure to assign them on a package level instead of dynamically
// within a function to avoid low performance
func newNumberConverter(targetType reflect.Type) converterFunc {
	return func(number interface{}) (interface{}, error) {
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

// making this package level so it is only evaluated once
var uint32Converter = newNumberConverter(reflect.TypeOf(uint32(0)))

// ToUInt32 is a function for converting any numeric type
// into a uint32. This can easily result in a loss of information
// due to truncation, so be careful.
func ToUInt32(number interface{}) (uint32, error) {
	asInterface, err := uint32Converter(number)
	if err != nil {
		return 0, err
	}
	// no check for "ok" here, since we know it will work
	return asInterface.(uint32), nil
}

var intConverter = newNumberConverter(reflect.TypeOf(int(0)))

// ToInt is a function for converting any numeric type
// into an int. This can easily result in a loss of information
// due to truncation of floats.
func ToInt(number interface{}) (int, error) {
	asInterface, err := intConverter(number)
	if err != nil {
		return 0, err
	}
	// no check for "ok" here, since we know it will work
	return asInterface.(int), nil
}
