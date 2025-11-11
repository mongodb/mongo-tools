package generate

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/option"
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/evergreen-ci/shrub"
)

func (gen *Generator) AddMongodumpResmokeTasks() {
	gen.addResmokeTasks(MongodumpPassthrough)
	gen.addResmokeTasks(MongodumpFSM)
	gen.addMongodumpFuzzTasks()
}

func (gen *Generator) addResmokeTasks(kind TaskKind) {
	if gen.err != nil {
		return
	}

	if !gen.spec.shouldGenerateKind(kind) {
		return
	}

	var (
		suites   []*resmokeSuite
		skipRand option.Option[*rand.Rand]
	)
	switch kind {
	case Passthrough:
		suites = passthroughSuites
		skipRand = gen.spec.SkipSuitesRand
	case FSM:
		suites = fsmSuites
		skipRand = gen.spec.SkipSuitesRand
	case MongodumpPassthrough:
		suites = mongodumpPassthroughSuites
	case MongodumpFSM:
		suites = mongodumpFSMSuites
	default:
		gen.err = fmt.Errorf("bad kind passed to addResmokeTasks: %s", kind)
	}

	suitesByVersionCombination := make(map[string][]VersionCombination)
	for _, suite := range suites {
		for _, vc := range SupportedVersionCombinations {

			if !gen.spec.shouldIncludeTask(
				taskNameWithVersionCombination(suite.name, vc),
			) {
				continue
			}

			if !gen.spec.shouldIncludeVersion(vc) {
				continue
			}

			if shouldSkipResmokeTaskGenForVersionCombination(suite, vc) {
				continue
			}

			suitesByVersionCombination[suite.name] = append(
				suitesByVersionCombination[suite.name],
				vc,
			)
		}
	}

	for _, suite := range suites {
		versionCombos := maybeRemoveSomeVersionCombinations(
			suitesByVersionCombination[suite.name],
			skipRand,
			suite.name,
		)

		for _, bv := range versionCombos {
			gen.addResmokeSuiteForVersionCombination(suite, bv)
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

	if IsMongodumpTaskGen() && strings.Contains(suite.name, "oplog") &&
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

	variantName := ""
	if IsMongodumpTaskGen() {
		variantName = "mongodump-"
	}
	variantName = variantName + vc.Amazon2ARM64String()

	gen.cfg.Variant(variantName).
		AddTasks(task.Name)
}

type resmokeSuite struct {
	name                      string
	timeoutDur                time.Duration
	resmokeJobsMax            int
	srcVersionsToSkip         mapset.Set[versions.ServerVersion]
	shouldSkipForCrossVersion bool
	coverage                  bool
}

func (rs *resmokeSuite) timeoutMultiplier(m float64) *resmokeSuite {
	if rs.timeoutDur.Minutes() == 0 {
		panic("we should never have a resmoke suite with a zero timeoutDur field")
	}
	rs.timeoutDur = time.Duration(rs.timeoutDur.Minutes()*m) * time.Minute
	return rs
}

func (rs *resmokeSuite) maxJobs(n int) *resmokeSuite {
	rs.resmokeJobsMax = n
	return rs
}

func (rs *resmokeSuite) skipForCrossVersion() *resmokeSuite {
	rs.shouldSkipForCrossVersion = true
	return rs
}

func (rs *resmokeSuite) skipForSrcVersions(srcVersions ...versions.ServerVersion) *resmokeSuite {
	rs.srcVersionsToSkip = mapset.NewSet(srcVersions...)
	return rs
}
