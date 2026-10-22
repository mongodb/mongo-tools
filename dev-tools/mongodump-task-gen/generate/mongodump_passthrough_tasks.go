package generate

import (
	mapset "github.com/deckarep/golang-set/v3"
	"github.com/mongodb/mongo-tools/dev-tools/mongodump-task-gen/versions"
)

func mongodumpPassthrough(name string) *resmokeSuite {
	return &resmokeSuite{
		name:              name,
		timeoutDur:        defaultPassthroughTimeout,
		srcVersionsToSkip: mapset.NewSet[versions.ServerVersion](),
	}
}

var mongodumpPassthroughSuites = []*resmokeSuite{
	mongodumpPassthrough("ctc_rs_dump_jscore_passthrough"),

	mongodumpPassthrough("ctc_rs_dump_oplog_sleep_jscore_passthrough").
		skipForSrcVersions(versions.V70, versions.V80, versions.V83),

	mongodumpPassthrough("ctc_rs_dump_sleep_jscore_passthrough").
		skipForSrcVersions(versions.V70, versions.V80, versions.V83),
}
