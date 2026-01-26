package bsontools

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// PointerTooDeepError is like POSIX’s ENOTDIR error: it indicates that a
// nonfinal element in a document pointer is a scalar value.
type PointerTooDeepError struct {
	givenPointer []string

	elementType    bson.Type
	elementPointer []string
}

func (pe PointerTooDeepError) Error() string {
	return fmt.Sprintf(
		"cannot delete %#q from BSON doc because %#q is of a simple type (%s)",
		pe.givenPointer,
		pe.elementPointer,
		pe.elementType,
	)
}

// ReplaceInRaw “surgically” replaces one value in a BSON document with another.
// Its returned bool indicates whether the value was found.
//
// If any nonfinal node in the pointer is a scalar value (i.e., neither an array
// nor embedded document), a PointerTooDeepError is returned.
//
// Example usage (replaces /role/title):
//
//	ReplaceInRaw(&rawDoc, newRoleTitle, "role", "title")
func ReplaceInRaw(rawRef *bson.Raw, newValue bson.RawValue, pointer ...string) (bool, error) {
	return replaceOrRemoveInRaw(rawRef, &newValue, pointer)
}

// RemoveFromRaw is like ReplaceInRaw, but it removes the element.
func RemoveFromRaw(rawRef *bson.Raw, pointer ...string) (bool, error) {
	return replaceOrRemoveInRaw(rawRef, nil, pointer)
}

func replaceOrRemoveInRaw(rawRef *bson.Raw, replacement *bson.RawValue, pointer []string) (bool, error) {
	sizeFromHeader := int(binary.LittleEndian.Uint32(*rawRef))

	pos := 4
	for pos < len(*rawRef)-1 {
		el, _, ok := bsoncore.ReadElement((*rawRef)[pos:])
		if !ok {
			return false, fmt.Errorf("invalid BSON element at offset %d", pos)
		}

		keyBytes := el.KeyBytes()

		valueAt := pos + 1 + len(keyBytes) + 1

		if string(keyBytes) != pointer[0] {
			pos += len(el)

			continue
		}

		bsonType := bson.Type(el[0])
		valueSize := len(el) - len(keyBytes) - 2

		var bytesAdded int

		// If this is the last node in the doc pointer, then remove/replace
		// the element.
		if len(pointer) == 1 {
			if replacement == nil {
				*rawRef = slices.Delete(*rawRef, pos, valueAt+valueSize)
				bytesAdded = -valueSize - len(keyBytes) - 2
			} else {
				(*rawRef)[pos] = byte(replacement.Type)

				bytesAdded = len(replacement.Value) - valueSize

				*rawRef = slices.Replace(
					*rawRef,
					valueAt,
					valueAt+valueSize,
					replacement.Value...,
				)
			}
		} else {
			if bsonType != bson.TypeArray && bsonType != bson.TypeEmbeddedDocument {
				return false, PointerTooDeepError{
					givenPointer:   slices.Clone(pointer),
					elementType:    bsonType,
					elementPointer: slices.Clone(pointer[:1]),
				}
			}

			curDoc := (*rawRef)[valueAt:]
			oldDocSize := int(binary.LittleEndian.Uint32(curDoc))

			found, err := replaceOrRemoveInRaw(&curDoc, replacement, pointer[1:])
			if err != nil {
				var pe PointerTooDeepError
				if errors.As(err, &pe) {
					pe.givenPointer = append(
						slices.Clone(pointer[:1]),
						pe.givenPointer...,
					)
					pe.elementPointer = append(
						slices.Clone(pointer[:1]),
						pe.elementPointer...,
					)

					return false, pe
				}
			}

			if !found {
				return false, nil
			}

			newDocSize := int(binary.LittleEndian.Uint32(curDoc))

			bytesAdded = newDocSize - oldDocSize

			*rawRef = append(
				(*rawRef)[:valueAt],
				curDoc...,
			)
		}

		binary.LittleEndian.PutUint32(*rawRef, uint32(sizeFromHeader+bytesAdded))

		return true, nil
	}

	return false, nil
}
