package safecast

import (
	"errors"
	"fmt"
)

// ErrConversionIssue is a generic error for type conversion issues
// It is used to wrap other errors
var ErrConversionIssue = errors.New("conversion issue")

// ErrRangeOverflow is an error for when the value is outside the range of the desired type
var ErrRangeOverflow = errors.New("range overflow")

// ErrExceedMaximumValue is an error for when the value is greater than the maximum value of the desired type.
var ErrExceedMaximumValue = errors.New("maximum value for this type exceeded")

// ErrExceedMinimumValue is an error for when the value is less than the minimum value of the desired type.
var ErrExceedMinimumValue = errors.New("minimum value for this type exceeded")

// ErrUnsupportedConversion is an error for when the conversion is not supported from the provided type.
var ErrUnsupportedConversion = errors.New("unsupported type")

// ErrStringConversion is an error for when the conversion fails from string.
var ErrStringConversion = errors.New("cannot convert from string")

// errorHelper is a helper struct for error messages
// It is used to wrap other errors, and provides additional information
type errorHelper[NumOut Number] struct {
	value any
	err   error
}

func (e errorHelper[NumOut]) Error() string {
	errMessage := ErrConversionIssue.Error()

	switch {
	case errors.Is(e.err, ErrExceedMaximumValue):
		boundary := maxOf[NumOut]()
		errMessage = fmt.Sprintf("%s: %v (%T) is greater than %v (%T)", errMessage, e.value, e.value, boundary, boundary)
	case errors.Is(e.err, ErrExceedMinimumValue):
		boundary := minOf[NumOut]()
		errMessage = fmt.Sprintf("%s: %v (%T) is less than %v (%T)", errMessage, e.value, e.value, boundary, boundary)
	case errors.Is(e.err, ErrUnsupportedConversion):
		errMessage = fmt.Sprintf("%s: %v (%T) is not supported", errMessage, e.value, e.value)
	case errors.Is(e.err, ErrStringConversion):
		targetType := NumOut(0)
		return fmt.Sprintf("%s: cannot convert from string %s to %T (base auto-detection)", errMessage, e.value, targetType)
	}

	if e.err != nil {
		errMessage = fmt.Sprintf("%s: %s", errMessage, e.err.Error())
	}
	return errMessage
}

func (e errorHelper[NumOut]) Unwrap() []error {
	errs := []error{ErrConversionIssue}
	if e.err != nil {
		switch {
		case
			errors.Is(e.err, ErrExceedMaximumValue),
			errors.Is(e.err, ErrExceedMinimumValue):
			errs = append(errs, ErrRangeOverflow)
		}
		errs = append(errs, e.err)
	}
	return errs
}
