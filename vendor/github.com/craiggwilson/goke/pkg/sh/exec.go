package sh

import (
	"bytes"
	"os"
	"os/exec"
	"strings"

	"github.com/craiggwilson/goke/task"
)

// ExitCode retrieves the exit code from an error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	if eerr, ok := err.(*exec.ExitError); ok {
		return eerr.ExitCode()
	}

	return 1
}

// IsNotRan indicates if command that generated the error actually ran.
func IsNotRan(err error) bool {
	if err == nil {
		return false
	}
	if eerr, ok := err.(*exec.ExitError); ok {
		return !eerr.Exited()
	}
	return true
}

// Run the specified command piping its output to goke's output.
func Run(ctx *task.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	return RunCmd(ctx, cmd)
}

// RunOutput runs the specified command and get the command output.
func RunOutput(ctx *task.Context, name string, args ...string) (string, error) {
	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &output
	err := RunCmd(ctx, cmd)
	return strings.TrimRight(output.String(), "\r\n"), err
}

// RunBuffered runs the specified command and returns the actual command exectued, stdout, and stderr.
func RunBuffered(ctx *task.Context, name string, args ...string) (string, string, string, error) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	return cmd.String(), stdout.String(), stderr.String(), err
}

// RunCmd runs the provided command.
func RunCmd(ctx *task.Context, cmd *exec.Cmd) error {
	LogCmd(ctx, cmd)
	if ctx.Verbose && cmd.Stdout == nil {
		cmd.Stdout = ctx
	}
	if cmd.Stderr == nil {
		cmd.Stderr = ctx
	}
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	return cmd.Run()
}
