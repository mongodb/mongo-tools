package task

import "fmt"

// Task represents a task to be executed
type Task interface {
	ContinueOnError() bool
	DeclaredArgs() []DeclaredTaskArg
	Dependencies() []string
	Description() string
	Executor() Executor
	Hidden() bool
	Name() string
}

type sortedTasks []Task

func (a sortedTasks) Len() int           { return len(a) }
func (a sortedTasks) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortedTasks) Less(i, j int) bool { return a[i].Name() < a[j].Name() }

// Validator validates arguments.
type Validator func(string, string) error

// Required is a validator that ensures that an argument is present.
var Required = Validator(func(name, s string) error {
	if s == "" {
		return fmt.Errorf("argument %q is required, but was not supplied", name)
	}

	return nil
})

// ChainValidator is a validator that is the conjunction of the given validators.
func ChainValidator(validators ...Validator) Validator {
	return func(name, s string) error {
		for _, validator := range validators {
			if validator != nil {
				if err := validator(name, s); err != nil {
					return err
				}
			}
		}

		return nil
	}
}

// DeclaredTaskArg is an argument for a particular task.
type DeclaredTaskArg struct {
	Name      string
	Validator Validator
}
