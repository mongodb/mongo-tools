package generate

import (
	"log"
	"math/rand"
	"slices"

	"github.com/mongodb/mongo-tools/mongodump_passthrough/mongo-go/option"
)

func maybeRemoveSomeVersionCombinations(
	versionCombos []VersionCombination,
	skipRand option.Option[*rand.Rand],
	name string,
) []VersionCombination {
	r, isSome := skipRand.Get()
	if !isSome {
		return versionCombos
	}

	// This will round down, so if there are 3 combinations, we'll just skip 1.
	skipCount := len(versionCombos) / 2
	for range skipCount {
		i := r.Intn(len(versionCombos))
		log.Printf(
			"Skipping %s -> %s for %s",
			versionCombos[i].srcVersion,
			versionCombos[i].dstVersion,
			name,
		)
		versionCombos = slices.Delete(versionCombos, i, i+1)
	}

	return versionCombos
}
