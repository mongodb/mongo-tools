// Package options implements mongotop-specific command-line options.
package options

import ()

// Output options for mongotop
type Output struct {
	Locks bool `long:"locks" description:"Report on use of per-database locks"`
	Once  bool `long:"once" description:"Only output stats page once, then quit"`
}

func (self *Output) Name() string {
	return "output"
}

func (self *Output) PostParse() error {
	return nil
}

func (self *Output) Validate() error {
	return nil
}
