//go:build unit

package versions_test

import (
	"encoding/json"
	"testing"

	"github.com/mongodb/mongo-tools/dev-tools/mongodump-task-gen/versions"
	"github.com/stretchr/testify/suite"
)

type UnitTestSuite struct {
	suite.Suite
}

func TestUnitTestSuite(t *testing.T) {
	ts := new(UnitTestSuite)
	suite.Run(t, ts)
}

type withPointer struct {
	Version *versions.ServerVersion `json:"version"`
}

type withoutPointer struct {
	Version versions.ServerVersion `json:"version"`
}

var v44JSON = []byte(`{"version":"4.4"}`)

func (s *UnitTestSuite) TestUnmarshal() {
	s.Run("non-pointer", func() {
		var v withoutPointer
		err := json.Unmarshal(v44JSON, &v)
		s.Require().NoError(err)
		s.Assert().Equal(versions.V44, v.Version, "unmarshal ok")
	})

	s.Run("pointer", func() {
		var v withPointer
		err := json.Unmarshal(v44JSON, &v)
		s.Require().NoError(err)
		s.Assert().Equal(versions.V44, *v.Version, "unmarshal ok")
	})
}

func (s *UnitTestSuite) TestMarshal() {
	s.Run("non-pointer", func() {
		v := withoutPointer{versions.V44}
		data, err := json.Marshal(v)
		s.Require().NoError(err)
		s.Assert().JSONEq(string(v44JSON), string(data), "marshal ok")
	})

	s.Run("pointer", func() {
		v := withPointer{&versions.V44}
		data, err := json.Marshal(v)
		s.Require().NoError(err)
		s.Assert().JSONEq(string(v44JSON), string(data), "marshal ok")
	})
}

func (s *UnitTestSuite) TestLatestSource() {
	s.Assert().Contains(
		versions.SupportedVersionPairsBySourceVersion,
		versions.LatestSource(),
	)
}
