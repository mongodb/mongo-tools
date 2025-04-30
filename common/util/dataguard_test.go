package util

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

type unitTestSuite struct {
	suite.Suite
}

func TestUnitTestSuite(t *testing.T) {
	suite.Run(t, &unitTestSuite{})
}

func (s *unitTestSuite) TestDataGuard() {
	l := NewDataGuard(42)

	var wg sync.WaitGroup
	for i := range 100 {
		var delta int
		if i%2 == 0 {
			delta = 2
		} else {
			delta = -1
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Store(func(v int) int {
				return v + delta
			})
		}()
	}
	wg.Wait()

	l.Load(func(v int) {
		s.Require().Equal(92, v)
	})

	s.Require().Equal(92, l.GetValue())
}
