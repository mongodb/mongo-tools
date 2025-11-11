package generate

// TODO: shyam - fix tests or remove
//
//import (
//	"context"
//	"fmt"
//	"os"
//	"path/filepath"
//	"regexp"
//	"slices"
//	"sort"
//	"strings"
//
//	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/git"
//	mapset "github.com/deckarep/golang-set/v2"
//	"github.com/evergreen-ci/shrub"
//	"github.com/samber/lo"
//)
//
//var (
//	versionsFromTaskName = regexp.MustCompile(`^(.*)_(v\d\d)_to_v\d\d$`)
//)
//
//func (s *UnitTestSuite) TestSuitesExist() {
//	gen := New(&Spec{})
//	gen.AddResmokeTasks()
//
//	repoRootDir, err := git.FindRepoRoot(context.Background())
//	s.Require().NoError(err)
//
//	taskNames := getResmokeTaskNamesSet().ToSlice()
//	sort.SliceStable(taskNames, func(a, b int) bool {
//		return taskNames[a] < taskNames[b]
//	})
//
//	for _, taskName := range taskNames {
//		// These are a weird special case where we generate tasks for them
//		// but don't run a suite.
//		if strings.Contains(taskName, "ctc_initial_sync_fuzzer_with_repl_dest") {
//			continue
//		}
//
//		var (
//			suiteRootDir string
//			suiteName    string
//		)
//		s.Require().Contains(taskName, "_to_")
//		matches := versionsFromTaskName.FindStringSubmatch(taskName)
//		s.Require().NotNil(matches, "task name %#q should have versions", taskName)
//
//		suiteName = taskName
//
//		switch matches[2] {
//		case "v44", "v50", "v60":
//			suiteRootDir = "resmoke/suite-config/6.0"
//		case "v70":
//			suiteRootDir = "resmoke/suite-config/7.0"
//		case "v80":
//			suiteRootDir = "resmoke/suite-config/8.0"
//		default:
//			panic("Task name has unknown source version: " + taskName)
//		}
//
//		testName := fmt.Sprintf("%#q in %#q", taskName, suiteRootDir)
//		s.Run(testName, func() {
//			suitePath := filepath.Join(repoRootDir, suiteRootDir, "suites", suiteName+".yml")
//			stat, err := os.Stat(suitePath)
//
//			s.Require().NoError(
//				err,
//				"The %#q task's suite (%s) should exist.",
//				taskName,
//				suitePath,
//			)
//			s.Require().False(stat.IsDir())
//		})
//	}
//}
//
//// This test ensures that there's an evergreen task for every resmoke suite.
//func (s *UnitTestSuite) TestTasksExist() {
//	taskNamesSet := getResmokeTaskNamesSet()
//
//	repoRootDir, err := git.FindRepoRoot(context.Background())
//	s.Require().NoError(err)
//
//	for _, versionStr := range []string{"6.0", "7.0", "8.0"} {
//		suites, err := os.ReadDir(
//			filepath.Join(repoRootDir, "resmoke/suite-config/", versionStr, "suites"),
//		)
//		s.Require().NoError(err)
//
//		suiteNames := lo.Map(
//			suites,
//			func(suite os.DirEntry, _ int) string {
//				return strings.TrimSuffix(suite.Name(), ".yml")
//			},
//		)
//
//		slices.Sort(suiteNames)
//
//		for _, suiteName := range suiteNames {
//			s.Require().Contains(suiteName, "_to_", "suite name must have versions")
//
//			s.Assert().Contains(
//				taskNamesSet.ToSlice(),
//				suiteName,
//				"task names should include suite name",
//			)
//		}
//	}
//}
//
//func getResmokeTaskNamesSet() mapset.Set[string] {
//	gen := New(&Spec{})
//	gen.AddResmokeTasks()
//
//	taskNames := mapset.NewSet(lo.Map(
//		gen.cfg.Tasks,
//		func(task *shrub.Task, _ int) string {
//			return task.Name
//		},
//	)...)
//
//	return taskNames
//}
