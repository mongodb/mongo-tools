package cli

import (
	"errors"
	"hash/fnv"
	"math"
	"math/rand"
	"regexp"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/task-generator/generate"
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/option"
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
)

var (
	useAllMongoVersions = false

	mongoVersionFlag = &cli.StringSliceFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "mongo version(s) to generate",
		Action:  validateMongoVersionFlags,
	}

	taskFlag = &cli.StringSliceFlag{
		Name:        "task",
		Aliases:     []string{"t"},
		Usage:       "task name(s) to generate",
		DefaultText: "all",
		Action:      validateTaskFlags,
	}

	bvFlag = &cli.BoolFlag{
		Name:    "buildvariants",
		Aliases: []string{"build-variants", "bv"},
		Usage:   "also generate the full resmoke build variants required",
	}

	variantFlag = &cli.StringFlag{
		Name:    "variant",
		Usage:   "the variant on which these tasks are being generated",
		EnvVars: []string{"EVG_VARIANT"},
	}

	dataraceOnlyFlag = &cli.BoolFlag{
		Name:  "dataraceOnly",
		Usage: "only generate datarace tasks for integration and e2e",
	}

	topologyFlag = &cli.StringFlag{
		Name:  "topology",
		Usage: "a regex for configuring which integration and e2e topologies to generate",
	}

	// We need this because of the way our Evergreen task config is set up. We have one task for
	// each type of thing we generate (passthroughs, fsm, e2e, etc.). Those tasks all call the
	// "generate evergreen tasks" function with different vars (args). Ideally, we'd only pass the
	// `--skipSeed` flag in these vars for patches or PR pushes. But there's no way to conditionally
	// change the content of vars.
	//
	// So instead we pass both the requester _and_ the seed. If the requester doesn't match certain
	// cases, we just ignore the `--skipSeed` flag.
	requesterFlag = &cli.StringFlag{
		Name:  "requester",
		Usage: "the Evergreen requester expansion; when this is either 'patch' or 'github_pr', the '--skipSeed' flag is respected; for other triggers, this is ignored",
	}

	skipSeedFlag = &cli.StringFlag{
		Name:  "skipSeed",
		Usage: "if set, the generator will skip some suites when generating fsm or passthrough tasks; takes a string used as a random seed when determining whether a suite should be skipped",
	}

	bvCommand = &cli.Command{
		Name:    "buildvariants",
		Usage:   "generate build variants",
		Aliases: []string{"variants", "bv"},
		Flags:   []cli.Flag{mongoVersionFlag},
		Action:  generateTaskKind(generate.BuildVariant),
	}

	fuzzCommand = &cli.Command{
		Name:    "jstestfuzz",
		Usage:   "generate jstestfuzz tasks",
		Aliases: []string{"fuzz", "fuzzer"},
		Flags:   []cli.Flag{mongoVersionFlag, taskFlag, bvFlag},
		Action:  generateTaskKind(generate.Fuzzer),
	}

	fsmCommand = &cli.Command{
		Name:  "fsm",
		Usage: "generate fsm tasks",
		Flags: []cli.Flag{
			mongoVersionFlag,
			taskFlag,
			bvFlag,
			requesterFlag,
			skipSeedFlag,
		},
		Action: generateTaskKind(generate.FSM),
	}

	passthroughCommand = &cli.Command{
		Name:    "passthrough",
		Usage:   "generate passthrough tasks",
		Aliases: []string{"pt", "passthru"},
		Flags: []cli.Flag{
			mongoVersionFlag,
			taskFlag,
			bvFlag,
			requesterFlag,
			skipSeedFlag,
		},
		Action: generateTaskKind(generate.Passthrough),
	}

	mongodumpFuzzCommand = &cli.Command{
		Name:    "mongodump_jstestfuzz",
		Usage:   "generate mongodump_jstestfuzz tasks",
		Aliases: []string{"mongodump_fuzz", "mongodump_fuzzer"},
		Flags:   []cli.Flag{mongoVersionFlag, taskFlag, bvFlag},
		Action:  generateTaskKind(generate.MongodumpFuzzer),
	}

	mongodumpFSMCommand = &cli.Command{
		Name:   "mongodump_fsm",
		Usage:  "generate mongodump_fsm tasks",
		Flags:  []cli.Flag{mongoVersionFlag, taskFlag, bvFlag},
		Action: generateTaskKind(generate.MongodumpFSM),
	}

	mongodumpPassthroughCommand = &cli.Command{
		Name:    "mongodump_passthrough",
		Usage:   "generate mongodump_passthrough tasks",
		Aliases: []string{"mongodump_pt", "mongodump_passthru"},
		Flags:   []cli.Flag{mongoVersionFlag, taskFlag, bvFlag},
		Action:  generateTaskKind(generate.MongodumpPassthrough),
	}

	upgradeCommand = &cli.Command{
		Name:   "upgrade",
		Usage:  "generate live binary upgrade tasks",
		Flags:  []cli.Flag{taskFlag},
		Action: generateTaskKind(generate.Upgrade),
	}

	ycsbCommand = &cli.Command{
		Name:   "ycsb",
		Usage:  "generate ycsb tasks",
		Action: generateTaskKind(generate.YCSB),
	}

	coverageCommand = &cli.Command{
		Name:   "coverage",
		Usage:  "generate coverage tasks",
		Action: generateTaskKind(generate.Coverage),
	}

	e2eCommand = &cli.Command{
		Name:  "e2e",
		Usage: "generate e2e tasks",
		Flags: []cli.Flag{
			mongoVersionFlag,
			taskFlag,
			variantFlag,
			dataraceOnlyFlag,
			topologyFlag,
			requesterFlag,
			skipSeedFlag,
		},
		Action: withEnsureVariant(generateTaskKind(generate.E2E)),
	}

	integrationCommand = &cli.Command{
		Name:  "integration",
		Usage: "generate integration tasks",
		Flags: []cli.Flag{
			mongoVersionFlag,
			taskFlag, variantFlag,
			dataraceOnlyFlag,
			topologyFlag,
			requesterFlag,
		},
		Action: withEnsureVariant(generateTaskKind(generate.Integration)),
	}

	errNoVariant = errors.New(
		"this command requires a variant; please set a variant using the --variant flag or the EVG_VARIANT environment variable",
	)
)

var App = &cli.App{
	Name:   "generate",
	Usage:  "generate evergreen tasks",
	Action: generateAllTasks, // root action, do everything
	Commands: []*cli.Command{
		bvCommand,
		fsmCommand,
		fuzzCommand,
		passthroughCommand,
		upgradeCommand,
		ycsbCommand,
		coverageCommand,
		e2eCommand,
		integrationCommand,
	},
}

// Initialize differently for mongodump-task-gen.
func InitForMongodumpTaskGen() {
	App = &cli.App{
		Name:   "generate",
		Usage:  "generate evergreen tasks",
		Action: generateAllTasks, // root action, do everything
		Commands: []*cli.Command{
			bvCommand,
			mongodumpFSMCommand,
			mongodumpFuzzCommand,
			mongodumpPassthroughCommand,
		},
	}
}

func withEnsureVariant(f cli.ActionFunc) cli.ActionFunc {
	return func(cctx *cli.Context) error {
		if cctx.String("variant") == "" {
			return errNoVariant
		}
		return f(cctx)
	}
}

func generateAllTasks(_ *cli.Context) error {
	spec := &generate.Spec{}
	return spec.Generate()
}

func generateTaskKind(kind generate.TaskKind) cli.ActionFunc {
	return func(cctx *cli.Context) error {
		return specFromFlags(cctx, kind).Generate()
	}
}

func specFromFlags(cctx *cli.Context, kind generate.TaskKind) *generate.Spec {
	requester := cctx.String("requester")
	if requester == "github_merge_queue" {
		return &generate.Spec{
			SkipAll: true,
		}
	}

	var taskRegexes []*regexp.Regexp
	for _, re := range cctx.StringSlice("task") {
		// The validator already confirmed that these actually _do_ compile.
		taskRegexes = append(taskRegexes, regexp.MustCompile(re))
	}

	topologyRegex := option.None[*regexp.Regexp]()
	if cctx.IsSet("topology") {
		topologyRegex = option.Some(
			regexp.MustCompile(cctx.String("topology")),
		)
	}

	// The point of this is to allow us to accept "latest" and turn it into an actual version.
	requestedVersions := lo.Map(
		cctx.StringSlice("version"),
		func(requestedVersion string, _ int) string {
			v, err := versions.ParseSupportedServerVersion(requestedVersion)
			// This should be impossible since we already validate the requested versions by parsing
			// them.
			if err != nil {
				panic(err)
			}
			return v.String()
		},
	)
	if useAllMongoVersions {
		requestedVersions = nil
	}

	var skipRand option.Option[*rand.Rand]

	if requester == "patch" || requester == "github_pr" {
		if seed := cctx.String("skipSeed"); seed != "" {
			seedHash := fnv.New64()
			seedHash.Write([]byte(seed))
			// The hash algorithm gives us a `uint64`, but `rand.NewSource` takes an `int64`.
			//
			//#nosec G115
			seed := int64(seedHash.Sum64() - math.MaxInt64)
			//#nosec G404
			skipRand = option.Some(rand.New(rand.NewSource(seed)))
		}
	}

	return &generate.Spec{
		Kinds:          []generate.TaskKind{kind},
		TaskRegexes:    taskRegexes,
		Versions:       requestedVersions,
		GenFullBVs:     cctx.Bool("buildvariants"),
		Variant:        cctx.String("variant"),
		DataraceOnly:   cctx.Bool("dataraceOnly"),
		TopologyRegex:  topologyRegex,
		SkipSuitesRand: skipRand,
	}
}
