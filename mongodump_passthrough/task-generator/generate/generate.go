package generate

import (
	"encoding/json"
	"fmt"

	"github.com/evergreen-ci/shrub"
)

type TaskKind string

const (
	BuildVariant         TaskKind = "buildvariant"
	Fuzzer               TaskKind = "fuzzer"
	Passthrough          TaskKind = "passthrough"
	Coverage             TaskKind = "coverage"
	FSM                  TaskKind = "fsm"
	Upgrade              TaskKind = "upgrade"
	YCSB                 TaskKind = "ycsb"
	E2E                  TaskKind = "e2e"
	Integration          TaskKind = "integration"
	MongodumpFuzzer      TaskKind = "mongodump_fuzzer"
	MongodumpPassthrough TaskKind = "mongodump_passthrough"
	MongodumpFSM         TaskKind = "mongodump_fsm"
)

type Generator struct {
	spec *Spec
	cfg  *shrub.Configuration
	err  error
}

func New(spec *Spec) *Generator {
	return &Generator{
		spec: spec,
		cfg:  &shrub.Configuration{},
	}
}

func (gen *Generator) Generate() error {
	gen.AddResmokeBuildVariants()

	if IsMongodumpTaskGen() {
		gen.AddMongodumpResmokeTasks()
	} else {
		gen.AddResmokeTasks()
		gen.AddUpgradeTasks()
		gen.AddYCSBTasks()
		gen.AddCoverageTasks()
		gen.AddE2ETasks()
		gen.AddIntegrationTasks()
	}

	if gen.err != nil {
		return gen.err
	}

	generated, err := json.MarshalIndent(gen.cfg, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(generated))
	return nil
}
