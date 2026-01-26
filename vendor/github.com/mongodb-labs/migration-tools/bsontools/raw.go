package bsontools

import (
	"fmt"
	"iter"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// RawLookup extracts & unmarshals a referent value from a BSON document.
// It’s like bson.Raw.LookupErr combined with RawValueTo.
func RawLookup[T unmarshalTargets](doc bson.Raw, pointer ...string) (T, error) {
	rv, err := doc.LookupErr(pointer...)

	if err != nil {
		return *new(T), fmt.Errorf("extracting %#q: %w", pointer, err)
	}

	val, err := RawValueTo[T](rv)

	if err != nil {
		return *new(T), fmt.Errorf("casting %#q: %w", pointer, err)
	}

	return val, err
}

// RawElements returns an iterator over a Raw’s elements.
//
// If the iterator returns an error but the caller continues iterating,
// a panic will ensue.
func RawElements(doc bson.Raw) iter.Seq2[bson.RawElement, error] {
	remaining := doc[4:]

	return func(yield func(bson.RawElement, error) bool) {
		var el bsoncore.Element
		var ok bool

		for len(remaining) > 1 {
			el, remaining, ok = bsoncore.ReadElement(remaining)

			var err error

			if !ok {
				err = bsoncore.NewInsufficientBytesError(doc, remaining)
			} else {
				err = el.Validate()
			}

			if err != nil {
				if yield(nil, err) {
					panic(fmt.Sprintf("Must stop iteration after error (%v)", err))
				}

				return
			}

			if !yield(bson.RawElement(el), nil) {
				return
			}
		}

	}
}
