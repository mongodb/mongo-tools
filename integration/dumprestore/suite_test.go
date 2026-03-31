package dumprestore

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	integrationSuite "github.com/mongodb/mongo-tools/integration/suite"
	"github.com/stretchr/testify/suite"
)

type DumpRestoreSuite struct {
	integrationSuite.IntegrationSuite
}

func TestDumpRestore(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	suite.Run(t, new(DumpRestoreSuite))
}
