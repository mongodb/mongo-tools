package generate

import (
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/option"
	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/versions"
	"math/rand"
	"regexp"
	"slices"
)

// Spec describes what tasks to generate.
type Spec struct {
	Kinds          []TaskKind       // If empty, generate all.
	TaskRegexes    []*regexp.Regexp // If empty, generate all.
	Versions       []string         // If empty, generate all.
	GenFullBVs     bool             // If true, generate the full build variant spec.
	Variant        string
	DataraceOnly   bool
	TopologyRegex  option.Option[*regexp.Regexp]
	SkipSuitesRand option.Option[*rand.Rand]
	SkipAll        bool
}

var variantMaxServerVersion = map[string]versions.ServerVersion{
	"rhel70": versions.V70,
}

func (spec *Spec) Generate() error {
	gen := New(spec)
	return gen.Generate()
}

func (spec *Spec) shouldGenerateKind(kind TaskKind) bool {
	if spec.SkipAll {
		// This is admittedly weird. By default, we generate everything, but also we want to be able to
		// generate _nothing_ in some cases, so this flag is the most expedient way to do that.
		return false
	}

	if len(spec.Kinds) == 0 {
		return true
	}

	return slices.Contains(spec.Kinds, kind)
}

func (spec *Spec) shouldIncludeTask(name string) bool {
	if len(spec.TaskRegexes) == 0 {
		return true
	}

	return slices.ContainsFunc(spec.TaskRegexes, func(re *regexp.Regexp) bool {
		return re.MatchString(name)
	})
}

func (spec *Spec) shouldIncludeVersion(bv VersionCombination) bool {
	if maxServerVersion, has := variantMaxServerVersion[spec.Variant]; has {
		for _, version := range [...]versions.ServerVersion{bv.srcVersion, bv.dstVersion} {
			if maxServerVersion.LessThan(version) {
				return false
			}
		}
	}

	if len(spec.Versions) == 0 {
		return true
	}

	return slices.ContainsFunc(spec.Versions, func(version string) bool {
		return bv.srcVersion.String() == version
	})
}
