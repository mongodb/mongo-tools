package shrub

import (
	"errors"
	"fmt"
)

// BuildConfiguration provides an interface for building configuration
// objects with some additional safety. The fluent interface for
// Configuration objects can panic in some situations, and you can use
// BuildConfiguration to convert these panics into errors that you can
// handle conventionally.
func BuildConfiguration(f func(*Configuration)) (c *Configuration, err error) {
	defer func() {
		if p := recover(); p != nil {
			c = nil
			switch pm := p.(type) {
			case error:
				err = pm
			case fmt.Stringer:
				err = errors.New(pm.String())
			case string:
				err = errors.New(pm)
			default:
				err = fmt.Errorf("%v", pm)
			}
		}
	}()

	c = &Configuration{}

	f(c)

	return
}
