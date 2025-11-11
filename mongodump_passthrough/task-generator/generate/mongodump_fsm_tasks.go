package generate

import (
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
	mapset "github.com/deckarep/golang-set/v2"
)

func mongodumpFSM(name string) *resmokeSuite {
	return &resmokeSuite{
		name:              name,
		timeoutDur:        defaultFSMTimeout,
		srcVersionsToSkip: mapset.NewSet[versions.ServerVersion](),
	}
}

// TODO: The suite timeout factors are derived from mongosync-related code -
// some of these were adjusted as part of REP-4526 to account
// for the 2 minute delay when mongosync restarts or resumes, so we might
// be able to use shorter timeouts for mongodump testing if necessary.

var mongodumpFSMSuites = []*resmokeSuite{
	mongodumpFSM("ctc_rs_dump_archive_custom_fsm"),

	mongodumpFSM("ctc_rs_dump_archive_oplog_custom_fsm"),

	mongodumpFSM("ctc_rs_dump_archive_oplog_kill_primary_concurrency_fsm").
		timeoutMultiplier(1.20).
		maxJobs(6).
		skipForSrcVersions(versions.V70, versions.V80),

	mongodumpFSM("ctc_rs_dump_concurrency_fsm").
		timeoutMultiplier(0.75),

	mongodumpFSM("ctc_rs_dump_custom_fsm"),

	mongodumpFSM("ctc_rs_dump_custom_fsm_collation"),

	mongodumpFSM("ctc_rs_dump_kill_primary_concurrency_fsm").
		timeoutMultiplier(1.20).
		maxJobs(6).
		skipForSrcVersions(versions.V70, versions.V80),

	mongodumpFSM("ctc_rs_dump_kill_primary_custom_fsm").
		timeoutMultiplier(1.20).
		maxJobs(6).
		skipForSrcVersions(versions.V70, versions.V80),

	mongodumpFSM("ctc_rs_dump_kill_primary_custom_fsm_collation").
		timeoutMultiplier(1.20).
		maxJobs(4).
		skipForSrcVersions(versions.V70, versions.V80),

	mongodumpFSM("ctc_rs_dump_oplog_concurrency_fsm"),

	mongodumpFSM("ctc_rs_dump_oplog_custom_fsm"),
}
