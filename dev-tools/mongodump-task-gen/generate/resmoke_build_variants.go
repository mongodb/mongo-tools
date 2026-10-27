package generate

import (
	"github.com/evergreen-ci/shrub"
)

// amazon2ARMGravitonDistro is a special distro that runs Amazon 2 with an ARM64 processor on an
// 8xlarge EC2 machine. All other ARM64 distros called "xlarge" use 16xlarge EC2 machines. We use
// this particular distro since it's cheaper than an equivalent Intel EC2 machine.
const amazon2ARMGravitonDistro = "amazon2-arm64-graviton3"

func (gen *Generator) addResmokeBuildVariants() {
	for _, versionCombination := range SupportedVersionCombinations {
		// Variant names for mongodump passthrough tests should start with "mongodump"
		// to simplify steering BFs to the right Build Baron context.
		variantName := "mongodump-" + versionCombination.Amazon2ARM64String()

		// These variants only run on patches and ad-hoc requests, matching how the passthrough
		// tests ran when they were generated at runtime.
		bv := gen.cfg.Variant(variantName).
			DisplayName(versionCombination.Amazon2ARM64DisplayName()).
			AllowedRequester("ad_hoc", "patch").
			Module("migration-verifier").
			// Mongodump passthroughs run from the mongo-tools repo and evergreen project,
			// and have to git.fetch mongosync as an evergreen module.  So each variant
			// needs to declare mongosync in its list of modules, otherwise it won't be
			// cloned.
			Module("mongosync").
			// There are a number of other Amazon 2 ARM64 distros, but in all other cases, their
			// "xlarge" is a 16xlarge EC2 instance type, not 8xlarge. That costs _more_ than using
			// an 8xlarge Intel EC2. This is the only ARM64 distro that uses an 8xlarge.
			RunOn(amazon2ARMGravitonDistro).
			Expansion("src_mongo_version", versionCombination.srcVersion.VString()).
			Expansion("dst_mongo_version", versionCombination.dstVersion.VString()).
			Expansion("src_pinned_server_version", versionCombination.srcVersion.GetServerPin()).
			Expansion("dst_pinned_server_version", versionCombination.dstVersion.GetServerPin()).
			Expansion("pinned_resmoke_version", versionCombination.srcVersion.GetResmokePin()).
			Expansion("resmoke_platform", getResmokeVariant()).
			Expansion("server_architecture", "aarch64").
			Expansion("server_platform", "amazon2").
			Expansion("mongosync_compile_build_variant", getResmokeVariant())

		bv.TaskSpec(shrub.TaskSpec{
			Name:  "t_resmoke_setup",
			RunOn: []string{"amazon2-arm64-latest-small"},
		})
	}
}
