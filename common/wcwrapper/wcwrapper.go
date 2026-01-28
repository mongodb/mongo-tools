package wcwrapper

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

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

func Wrap(base *writeconcern.WriteConcern) *WriteConcern {
	return &WriteConcern{WriteConcern: base}
}

func Majority() *WriteConcern {
	return Wrap(writeconcern.Majority())
}
