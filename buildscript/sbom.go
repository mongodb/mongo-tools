package buildscript

import (
	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
)

func WriteSBOMLite(ctx *task.Context) error {
	return sh.Run(ctx, "scripts/regenerate-sbom-lite.sh")
}
