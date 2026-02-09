// Package wcwrapper exists so that we can wrap the Go driver's writeconcern.WriteConcern type to
// provide a WriteConcern with a timeout. See the documentation for the WriteConcern type for more
// details.
package wcwrapper

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// WriteConcern wraps the Go driver's standard WriteConcern type and adds a WTimeout field. This was
// added when we upgraded the Go driver to v2, which removed the WTimeout field from its
// writeconcern with the guidance to use contexts instead. This type exists so that we can do that
// relatively easily by stashing a provided WTimeout value along with the ordinary write concern. The
// tools can take a --writeConcern flag that contains a JSON string that might include wtimeout, so
// by using this wrapper type we can create a context with the correct timeout to match. (The
// alternative would be to pass a WTimeout value around separately, but we only need that value
// where we already need the ordinary WriteConcern anwyay.)
type WriteConcern struct {
	*writeconcern.WriteConcern
	WTimeout time.Duration
}

// New returns an empty WriteConcern.
func New() *WriteConcern {
	return &WriteConcern{
		WriteConcern: new(writeconcern.WriteConcern),
	}
}

// Wrap returns a WriteConcern that wraps the provided driver-standard writeconcern.WriteConcern.
func Wrap(base *writeconcern.WriteConcern) *WriteConcern {
	return &WriteConcern{WriteConcern: base}
}

// Majority is a convenience function that returns a wrapped majority write concern.
func Majority() *WriteConcern {
	return Wrap(writeconcern.Majority())
}

// MarshalBSON implements the bson.Marshaler interface.
func (wc *WriteConcern) MarshalBSON() ([]byte, error) {
	if wc == nil || wc.WriteConcern == nil {
		return nil, fmt.Errorf("cannot marshal an empty WriteConcern")
	}

	concernDoc := bson.D{{"w", wc.W}}
	if wc.Journal != nil {
		concernDoc = append(concernDoc, bson.E{Key: "j", Value: *wc.Journal})
	}

	if wc.WTimeout > 0 {
		concernDoc = append(
			concernDoc,
			bson.E{Key: "wtimeout", Value: wc.WTimeout.Milliseconds()},
		)
	}

	return bson.Marshal(concernDoc)
}
