package generate

import (
	"github.com/evergreen-ci/shrub"
)

const Amazon2ARMGraviton = "amazon2-arm64-graviton3"

func (gen *Generator) AddResmokeBuildVariants() {
	if gen.err != nil {
		return
	}

	// Don't do this unless we're generating the build variants specifically,
	// _or_ we were instructed to generate the build variants even for
	// non-BuildVariant generation.
	if !gen.spec.shouldGenerateKind(BuildVariant) && !gen.spec.GenFullBVs {
		return
	}

	for _, versionCombination := range SupportedVersionCombinations {
		if !gen.spec.shouldIncludeVersion(versionCombination) {
			continue
		}

		// Variant names for mongodump passthrough tests should start with "mongodump"
		// to simplify steering BFs to the right Build Baron context.
		variantName := versionCombination.Amazon2ARM64String()
		compileVariant := "amazon2-arm64"

		if IsMongodumpTaskGen() {
			variantName = "mongodump-" + variantName
			compileVariant = "mongodump_passthru_v"
		}

		bv := gen.cfg.Variant(variantName).
			DisplayName(versionCombination.Amazon2ARM64DisplayName()).
			Module("migration-verifier")
		if IsMongodumpTaskGen() {
			// Mongodump passthroughs run from the mongo-tools repo and evergreen project,
			// and have to git.fetch mongosync as an evergreen module.  So each variant
			// needs to declare mongosync in its list of modules, otherwise it won't be
			// cloned.
			bv = bv.Module("mongosync")
		}
		// There are a number of other Amazon 2 ARM64 distros, but in all other cases, their
		// "xlarge" is a 16xlarge EC2 instance type, not 8xlarge. That costs _more_ than using
		// an 8xlarge Intel EC2. This is the only ARM64 distro that uses an 8xlarge.
		bv = bv.RunOn(Amazon2ARMGraviton).
			Expansion("src_mongo_version", versionCombination.srcVersion.VString()).
			Expansion("dst_mongo_version", versionCombination.dstVersion.VString()).
			Expansion("src_pinned_server_version", versionCombination.srcVersion.GetServerPin()).
			Expansion("dst_pinned_server_version", versionCombination.dstVersion.GetServerPin()).
			Expansion("pinned_resmoke_version", versionCombination.srcVersion.GetResmokePin()).
			Expansion("resmoke_platform", compileVariant).
			Expansion("server_architecture", "aarch64").
			Expansion("server_platform", "amazon2").
			Expansion("mongosync_compile_build_variant", compileVariant)

		bv.TaskSpec(shrub.TaskSpec{
			Name:   "t_resmoke_setup",
			Distro: []string{"amazon2-arm64-xsmall"},
		})
	}
}
