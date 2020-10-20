package build

import (
	"os"
	"os/exec"

	"github.com/craiggwilson/goke/pkg/golang"
	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
)

func Registry() *task.Registry {
	registry := task.NewRegistry(task.WithAutoNamespaces(true))
	registry.Declare("build").Description("build the goke build script").DependsOn("clean", "sa").Do(Build)
	registry.Declare("clean").Description("cleans up the artifacts").Do(Clean)
	registry.Declare("sa:lint").Description("lint the packages").Do(Lint)
	registry.Declare("sa:fmt").Description("formats the packages").Do(Fmt)
	registry.Declare("test").Description("runs tests in all the packages").Do(Test)
	return registry
}

func Build(ctx *task.Context) error {
	args := []string{"build", "-o", buildOutputFile}
	if ctx.Verbose {
		args = append(args, "-v")
	}

	args = append(args, mainFile)
	return sh.Run(ctx, "go", args...)
}

func Clean(ctx *task.Context) error {
	_ = os.Remove(buildOutputFile)
	return nil
}

func Fmt(ctx *task.Context) error {
	args := []string{"-s", "-l"}
	if ctx.Verbose {
		args = append(args, "-e")
	}

	args = append(args, mainFile)
	return sh.Run(ctx, "gofmt", args...)
}

func Lint(ctx *task.Context) error {
	args := []string{"-set_exit_status"}
	args = append(args, packages...)
	return sh.Run(ctx, "golint", args...)
}

func Test(ctx *task.Context) error {
	args := []string{"test"}
	if ctx.Verbose {
		args = append(args, "-v")
	}
	args = append(args, packages...)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stdout = golang.ColoredTestWriter(ctx)
	cmd.Stderr = golang.ColoredTestWriter(ctx)

	return sh.RunCmd(ctx, cmd)
}
