package task

import (
	"context"
	"fmt"
	"io"
	"os"
)

// NewContext makes a new Context.
func NewContext(ctx context.Context, w io.Writer, taskArgs map[string]string, params ...ContextParam) *Context {
	c := &Context{
		Context:  ctx,
		w:        w,
		taskArgs: taskArgs,
	}
	for _, param := range params {
		param(c)
	}
	return c
}

type ContextParam = func(ctx *Context)

func WithVerbose(verbose bool) ContextParam {
	return func(ctx *Context) {
		ctx.Verbose = verbose
	}
}

func WithUI(ui *TUI) ContextParam {
	return func(ctx *Context) {
		ctx.UI = ui
	}
}

// Context holds information relevant to executing tasks.
type Context struct {
	context.Context

	UI      *TUI
	Verbose bool

	taskArgs map[string]string
	w        io.Writer
}

// Get returns an argument of the given name. If one doesn't exist,
// a lookup in the environment will be made.
func (ctx *Context) Get(name string) string {
	if ctx.taskArgs != nil {
		if v, ok := ctx.taskArgs[name]; ok {
			return v
		}
	}

	return os.Getenv(name)
}

// Log formats using the default formats for its operands sends it to the log.
// Spaces are added between operands when neither is a string.
func (ctx *Context) Log(v ...interface{}) {
	_, _ = fmt.Fprint(ctx.w, v...)
}

// Logln formats using the default formats for its operands and sends it to the log.
// Spaces are always added between operands and a newline is appended.
func (ctx *Context) Logln(v ...interface{}) {
	_, _ = fmt.Fprintln(ctx.w, v...)
}

// Logf formats according to a format specifier and sends it to the log.
func (ctx *Context) Logf(format string, v ...interface{}) {
	_, _ = fmt.Fprintf(ctx.w, format, v...)
}

// Writer implements the io.Writer interface.
func (ctx *Context) Write(p []byte) (n int, err error) {
	return ctx.w.Write(p)
}

// CopyArgs returns a copy of the current context's task arguments.
func (ctx *Context) CopyArgs() map[string]string {
	cpy := make(map[string]string, len(ctx.taskArgs))
	for k, v := range ctx.taskArgs {
		cpy[k] = v
	}
	return cpy
}
