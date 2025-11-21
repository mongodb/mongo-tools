package generate

import (
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
	"time"
)

const defaultPassthroughTimeout = 60 * time.Minute

func mongodumpPassthrough(name string) *resmokeSuite {
	return &resmokeSuite{
		name:              name,
		timeoutDur:        defaultPassthroughTimeout,
		srcVersionsToSkip: mapset.NewSet[versions.ServerVersion](),
		coverage:          false,
	}
}

var mongodumpPassthroughSuites = []*resmokeSuite{
	mongodumpPassthrough("ctc_rs_dump_archive_oplog_stepdown_jscore_passthrough"),

	mongodumpPassthrough("ctc_rs_dump_disconnect_jscore_passthrough"),

	mongodumpPassthrough("ctc_rs_dump_jscore_passthrough"),

	mongodumpPassthrough("ctc_rs_dump_kill_primary_jscore_passthrough").
		timeoutMultiplier(0.33),

	mongodumpPassthrough("ctc_rs_dump_oplog_sleep_jscore_passthrough").
		skipForSrcVersions(versions.V70, versions.V80),

	mongodumpPassthrough("ctc_rs_dump_sleep_jscore_passthrough").
		skipForSrcVersions(versions.V70, versions.V80),

	mongodumpPassthrough("ctc_rs_dump_stepdown_jscore_passthrough").
		timeoutMultiplier(0.33),

	mongodumpPassthrough("ctc_rs_dump_terminate_primary_jscore_passthrough").
		timeoutMultiplier(0.33),
}
