package value

import "reflect"

// IsZero indicates whether the given value is its type’s zero value.
func IsZero[T any](specimen T) bool {
	// copied from samber/mo.EmptyableToOption:
	return reflect.ValueOf(&specimen).Elem().IsZero()
}
