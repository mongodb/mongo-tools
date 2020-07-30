package command

import (
	"os/exec"

	"github.com/craiggwilson/goke/pkg/sh"
	"github.com/craiggwilson/goke/task"
)

// Command wraps exec.Command in a task executor.
func Command(name string, args ...string) task.Executor {
	return func(ctx *task.Context) error {
		return sh.Run(ctx, name, args...)
	}
}

// Executor creates a task.Executor from the command.
func Executor(cmd *exec.Cmd) task.Executor {
	return func(ctx *task.Context) error {
		sh.LogCmd(ctx, cmd)
		return cmd.Run()
	}
}
