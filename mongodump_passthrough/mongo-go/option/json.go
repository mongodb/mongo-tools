package option

import (
	"bytes"
	"encoding/json"
)

var _ json.Marshaler = &Option[int]{}
var _ json.Unmarshaler = &Option[int]{}

// MarshalJSON encodes Option into json.
func (o Option[T]) MarshalJSON() ([]byte, error) {
	val, exists := o.Get()
	if exists {
		return json.Marshal(val)
	}

	return json.Marshal(nil)
}

// UnmarshalJSON decodes Option from json.
func (o *Option[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		o.val = nil
	} else {
		val := *new(T)

		err := json.Unmarshal(b, &val)
		if err != nil {
			return err
		}

		o.val = &val
	}

	return nil
}
