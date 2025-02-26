package mongorestore

import (
	"bytes"
	"slices"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var nineNuls = make([]byte, 9, 9)

// FindZeroTimestamps returns all document paths (each represented as
// a []string) that contain a zero-value timestamp.
func FindZeroTimestamps(raw bson.Raw) ([][]string, error) {
	if !bytesSuggestEmptyTimestamp(raw) {
		return nil, nil
	}

	// TODO: log(trace, "found empty-timestamp byte sequence")

	var findFields func(curField []string, val bson.RawValue) ([][]string, error)
	findFields = func(curField []string, val bson.RawValue) ([][]string, error) {
		if val.Type == bson.TypeTimestamp {
			t, i := val.Timestamp()

			if t == 0 && i == 0 {
				return [][]string{curField}, nil
			}

			return nil, nil
		}

		var doc bson.Raw

		// NB: BSON marshals arrays as if they were embedded documents
		// with stringified-number keys. We parse them accordingly.
		switch val.Type {
		case bson.TypeEmbeddedDocument, bson.TypeArray:
			doc = val.Value
		default:
			return nil, nil
		}

		els, err := doc.Elements()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse %+v", curField)
		}

		foundSubFields := [][]string{}

		for i, el := range els {
			key, err := el.KeyErr()
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to parse key from element %d of %+v",
					i,
					curField,
				)
			}

			subVal, err := el.ValueErr()
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to parse value from element %d of %+v",
					i,
					curField,
				)
			}

			subField := append(
				slices.Clone(curField),
				key,
			)

			newFields, err := findFields(subField, subVal)
			if err != nil {
				return nil, err
			}

			foundSubFields = append(foundSubFields, newFields...)
		}

		return foundSubFields, nil
	}

	return findFields(
		[]string{},
		bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: raw},
	)
}

func bytesSuggestEmptyTimestamp(raw bson.Raw) bool {
	// The pattern we seek is:
	//
	//	/\x11 [^\x00]* \x00 \x00{4} \x00{4}/x
	//
	// .. i.e., 0x11, then the NUL-terminated field name, then the timestamp’s
	// two zero-value int32s.
	//
	// For speed we examine raw bytes rather than using a regexp.

	var curRaw []byte
	tsAt := -1

	for {
		if curRaw == nil {
			curRaw = []byte(raw)
		} else {
			if tsAt == -1 {
				panic("tsAt should be nonnegative!")
			}

			curRaw = curRaw[1+tsAt:]
		}

		tsAt = bytes.IndexByte(curRaw, byte(bson.TypeTimestamp))

		// The most likely scenario: no timestamp byte in the document at all.
		if tsAt == -1 {
			return false
		}

		// OK, we found a timestamp byte. That means little on its own because
		// that byte is probably part of some other value. To narrow things
		// down let’s find the next 9-NUL sequence.
		bytesAfterTs := curRaw[1+tsAt:]
		nineNulsAt := bytes.Index(bytesAfterTs, nineNuls)

		// Most likely:
		if nineNulsAt == -1 {
			continue
		}

		// It’s getting more likely that we have an empty timestamp. To confirm,
		// we check to see if any NULs are between the 0x11 and the nine NULs.
		firstNulBetweenAt := bytes.IndexByte(bytesAfterTs[:nineNulsAt], 0x00)

		// Most likely: we find a NUL between the 0x11 and the nine NULs, which
		// means that we did not, in fact, find an empty timestamp.
		if firstNulBetweenAt != -1 {
			continue
		}

		// This means we found the sequence we sought. It could have appeared
		// inside some other field (e.g., binary string), but at this point
		// we do a full BSON decode to determine that.
		return true
	}
}
