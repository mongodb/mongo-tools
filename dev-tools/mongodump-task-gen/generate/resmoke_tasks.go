package generate

import (
	"fmt"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v3"
	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/mongo-tools/dev-tools/mongodump-task-gen/versions"
)

// getResmokeVariant is the variant that compiles the binaries the passthrough tasks depend on.
func getResmokeVariant() string {
	return "mongodump_passthru_v"
}

func (gen *Generator) addMongodumpResmokeTasks() {
	gen.addResmokeSuites(mongodumpPassthroughSuites)
	gen.addResmokeSuites(mongodumpFSMSuites)
}

func (gen *Generator) addResmokeSuites(suites []*resmokeSuite) {
	for _, suite := range suites {
		for _, vc := range SupportedVersionCombinations {
			if shouldSkipResmokeTaskGenForVersionCombination(suite, vc) {
				continue
			}

			gen.addResmokeSuiteForVersionCombination(suite, vc)
		}
	}
}

func shouldSkipResmokeTaskGenForVersionCombination(
	suite *resmokeSuite,
	versionCombination VersionCombination,
) bool {
	if suite.srcVersionsToSkip.Contains(versionCombination.srcVersion) {
		return true
	}
	if versionCombination.srcVersion != versionCombination.dstVersion &&
		suite.shouldSkipForCrossVersion {
		return true
	}

	if versionCombination.srcVersion.LessThan(versions.V60) {
		// Reverse is not supported with pre-6.0 sources.
		if strings.Contains(suite.name, "reverse") {
			return true
		}
	}

	if strings.Contains(suite.name, "oplog") &&
		!versionCombination.IsMongodumpWithOplogSupported() {
		return true
	}

	return false
}

func (gen *Generator) addResmokeSuiteForVersionCombination(
	suite *resmokeSuite,
	vc VersionCombination,
) {
	taskName := taskNameWithVersionCombination(suite.name, vc)

	task := gen.cfg.Task(taskName).
		Dependency(shrub.TaskDependency{Name: "t_resmoke_setup"}).
		ExecTimeout(int(suite.timeoutDur.Seconds()))

	task.Dependency(
		shrub.TaskDependency{
			Name:    "compile_coverage",
			Variant: getResmokeVariant(),
		},
	)

	resmoke_args := "--storageEngine=wiredTiger"
	if suite.name == "ctc_custom_replica_sets_fsm" && vc.srcVersion.AtLeast(versions.V60) {
		// Repeated tests generate DDLs which pre-6.0 sources don't support.
		resmoke_args += " --repeatTests=4"
	}

	task.AddCommand().
		Function("passthrough setup").
		Var("mongosync_binary_folder", "mongosync-coverage-binary")
	run := task.AddCommand().Function("run tests").
		Var("use_large_distro", "true").
		Var("resmoke_args", resmoke_args).
		Var("suite", suiteNameWithVersionCombinationSuffix(suite.name, vc))

	if suite.resmokeJobsMax != 0 {
		run.Var("resmoke_jobs_max", fmt.Sprint(suite.resmokeJobsMax))
	}

	gen.cfg.Variant("mongodump-" + vc.Amazon2ARM64String()).
		AddTasks(task.Name)
}

type resmokeSuite struct {
	name                      string
	timeoutDur                time.Duration
	resmokeJobsMax            int
	srcVersionsToSkip         mapset.Set[versions.ServerVersion]
	shouldSkipForCrossVersion bool
}

func (rs *resmokeSuite) timeoutMultiplier(m float64) *resmokeSuite {
	if rs.timeoutDur.Minutes() == 0 {
		panic("we should never have a resmoke suite with a zero timeoutDur field")
	}
	rs.timeoutDur = time.Duration(rs.timeoutDur.Minutes()*m) * time.Minute
	return rs
}

func (rs *resmokeSuite) skipForSrcVersions(srcVersions ...versions.ServerVersion) *resmokeSuite {
	rs.srcVersionsToSkip = mapset.NewSet(srcVersions...)
	return rs
}
