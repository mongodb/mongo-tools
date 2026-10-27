package generate

import "github.com/evergreen-ci/shrub"

// Generator builds the Evergreen configuration for the mongodump passthrough tasks.
type Generator struct {
	cfg *shrub.Configuration
}

func New() *Generator {
	return &Generator{cfg: &shrub.Configuration{}}
}

// Configuration builds and returns the full passthrough configuration: the per-version build
// variants and the passthrough, FSM, and fuzzer tasks that run on them.
func (gen *Generator) Configuration() *shrub.Configuration {
	gen.addResmokeBuildVariants()
	gen.addMongodumpResmokeTasks()

	return gen.cfg
}
