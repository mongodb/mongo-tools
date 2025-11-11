// Package option implements [option types] in Go.
// It takes inspiration from [samber/mo] but also works with BSON and exposes
// a (hopefully) more refined interface.
//
// Option types facilitate avoidance of nil-dereference bugs, at the cost of a
// bit more overhead.
//
// A couple special notes:
//   - nil values inside the Option, like `Some([]int(nil))`, are forbidden.
//   - Option’s BSON marshaling/unmarshaling interoperates with the [bson]
//     package’s handling of nilable pointers. So any code that uses nilable
//     pointers to represent optional values can switch to Option and
//     should continue working with existing persisted data.
//   - Because encoding/json provides no equivalent to bson.Zeroer,
//     Option always marshals to JSON null if empty.
//
// Prefer Option to nilable pointers in all new code, and consider
// changing existing code to use it.
//
// [option types]: https://en.wikipedia.org/wiki/Option_type

// TODO: shyam - can we get rid of this entirely and just use regular option?
package option

import (
	"fmt"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/value"
	//"go.mongodb.org/mongo-driver/v2/bson"
)

//var _ bson.ValueMarshaler = &Option[int]{}
//var _ bson.ValueUnmarshaler = &Option[int]{}
//var _ bson.Zeroer = &Option[int]{}

// Option represents a possibly-empty value.
// Its zero value is the empty case.
type Option[T any] struct {
	val *T
}

// Some creates an Option with a value.
func Some[T any](val T) Option[T] {
	if isNil(val) {
		panic(fmt.Sprintf("Option forbids nil value (%T).", val))
	}

	return Option[T]{&val}
}

// None creates an Option with no value.
//
// Note that `None[T]()` is interchangeable with `Option[T]{}`.
func None[T any]() Option[T] {
	return Option[T]{}
}

// FromPointer will convert a nilable pointer into its
// equivalent Option.
func FromPointer[T any](valPtr *T) Option[T] {
	if valPtr == nil {
		return None[T]()
	}

	if isNil(*valPtr) {
		panic(fmt.Sprintf("Given pointer (%T) refers to nil, which is forbidden.", valPtr))
	}

	myCopy := *valPtr

	return Option[T]{&myCopy}
}

// IfNotZero returns an Option that’s populated if & only if
// the given value is a non-zero value. (NB: The zero value
// for slices & maps is nil, not empty!)
//
// This is useful, e.g., to interface with code that uses
// nil to indicate a missing slice or map.
func IfNotZero[T any](val T) Option[T] {
	if value.IsZero(val) {
		return Option[T]{}
	}

	return Option[T]{&val}
}

// Get “unboxes” the Option’s internal value.
// The boolean indicates whether the value exists.
func (o Option[T]) Get() (T, bool) {
	if o.val == nil {
		return *new(T), false
	}

	return *o.val, true
}

// MustGet is like Get but panics if the Option is empty.
func (o Option[T]) MustGet() T {
	val, exists := o.Get()
	if !exists {
		panic(fmt.Sprintf("MustGet() called on empty %T", o))
	}

	return val
}

// OrZero returns either the Option’s internal value or
// the type’s zero value.
func (o Option[T]) OrZero() T {
	val, exists := o.Get()
	if exists {
		return val
	}

	return *new(T)
}

// OrElse returns either the Option’s internal value or
// the given `fallback`.
func (o Option[T]) OrElse(fallback T) T {
	val, exists := o.Get()
	if exists {
		return val
	}

	return fallback
}

// ToPointer converts the Option to a nilable pointer.
// The internal value (if it exists) is (shallow-)copied.
func (o Option[T]) ToPointer() *T {
	val, exists := o.Get()
	if exists {
		theCopy := val
		return &theCopy
	}

	return nil
}

// IsNone returns a boolean indicating whether or not the option is a None
// value.
func (o Option[T]) IsNone() bool {
	return o.val == nil
}

// IsSome returns a boolean indicating whether or not the option is a Some
// value.
func (o Option[T]) IsSome() bool {
	return o.val != nil
}
