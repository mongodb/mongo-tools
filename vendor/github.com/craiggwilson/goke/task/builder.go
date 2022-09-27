package task

// build begins building a task.
func build(name string) *Builder {
	task := &declaredTask{
		name: name,
	}
	return &Builder{task: task}
}

// Builder provides a fluent way to build up a task.
type Builder struct {
	task *declaredTask
}

// Arg declares an argument for the task.
func (b *Builder) Arg(name string, validator ...Validator) *Builder {
	if len(validator) != 0 {
		b.task.declaredArgs = append(b.task.declaredArgs, DeclaredTaskArg{
			Name:      name,
			Validator: ChainValidator(validator...),
		})
		return b
	}
	return b.OptionalArg(name)
}

// OptionalArg declares an optional argument for the task.
func (b *Builder) OptionalArg(name string) *Builder {
	b.task.declaredArgs = append(b.task.declaredArgs, DeclaredTaskArg{
		Name: name,
	})
	return b
}

// OptionalArgs declares optional arguments for the task.
func (b *Builder) OptionalArgs(names ...string) *Builder {
	for _, name := range names {
		b = b.OptionalArg(name)
	}
	return b
}

// RequiredArg declares a required argument for the task.
func (b *Builder) RequiredArg(name string) *Builder {
	b.task.declaredArgs = append(b.task.declaredArgs, DeclaredTaskArg{
		Name:      name,
		Validator: Required,
	})
	return b
}

// RequiredArgs declares required arguments for the task.
func (b *Builder) RequiredArgs(names ...string) *Builder {
	for _, name := range names {
		b = b.RequiredArg(name)
	}
	return b
}

// ContinueOnError declares that a task should not stop the build from continuing.
func (b *Builder) ContinueOnError() *Builder {
	b.task.continueOnError = true
	return b
}

// Description sets the description for the task.
func (b *Builder) Description(description string) *Builder {
	b.task.description = description
	return b
}

// DependsOn declares other tasks which must run before this one.
func (b *Builder) DependsOn(names ...string) *Builder {
	b.task.dependencies = names
	return b
}

// Do declares the executor when this task runs.
func (b *Builder) Do(executor Executor) {
	b.task.executor = executor
}

// Hide the task from the task list.
func (b *Builder) Hide() *Builder {
	b.task.hidden = true
	return b
}

type declaredTask struct {
	declaredArgs    []DeclaredTaskArg
	dependencies    []string
	name            string
	description     string
	executor        Executor
	continueOnError bool
	hidden          bool
}

func (t *declaredTask) ContinueOnError() bool {
	return t.continueOnError
}
func (t *declaredTask) DeclaredArgs() []DeclaredTaskArg {
	return t.declaredArgs
}
func (t *declaredTask) Dependencies() []string {
	return t.dependencies
}
func (t *declaredTask) Description() string {
	return t.description
}
func (t *declaredTask) Hidden() bool {
	return t.hidden
}
func (t *declaredTask) Executor() Executor {
	return t.executor
}
func (t *declaredTask) Name() string {
	return t.name
}
