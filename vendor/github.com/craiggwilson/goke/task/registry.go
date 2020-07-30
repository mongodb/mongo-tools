package task

import (
	"fmt"
	"sort"
	"strings"
)

// RegistryOption is an option for setting up a task registry.
type RegistryOption func(r *Registry)

// WithAutoNamespaces tells the registry whether to automatically create
// namespace tasks.
func WithAutoNamespaces(v bool) RegistryOption {
	return RegistryOption(func(r *Registry) {
		r.autoNS = v
	})
}

// WithNamespaceSeparator sets the separator for namespaces.
func WithNamespaceSeparator(s string) RegistryOption {
	return RegistryOption(func(r *Registry) {
		r.nsSeparator = s
	})
}

// NewRegistry creates a new registry.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		autoNS:      false,
		nsSeparator: ":",
	}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Registry holds all the tasks able to be run.
type Registry struct {
	tree        taskTree
	nsSeparator string
	autoNS      bool
}

// Tasks returns all the tasks and pseudo-tasks.
func (r *Registry) Tasks() []Task {
	var collect func(string, *taskTree) []Task
	collect = func(path string, tree *taskTree) []Task {
		var tasks []Task
		for _, child := range tree.children {
			newPath := child.name
			if path != "" {
				newPath = path + r.nsSeparator + child.name
			}
			tasks = append(tasks, collect(newPath, child)...)
		}

		if tree.task != nil {
			tasks = append(tasks, tree.task)
		} else if r.autoNS && path != "" {
			var deps []string
			for _, d := range tasks {
				if path == r.taskNamespace(d) {
					deps = append(deps, d.Name())
				}
			}

			builder := build(path).DependsOn(deps...)
			tasks = append(tasks, builder.task)
		}

		return tasks
	}

	tasks := sortedTasks(collect("", &r.tree))
	sort.Sort(tasks)
	return tasks
}

// Register a task in the Configuration.
func (r *Registry) Register(task Task) {
	r.registerTask(&r.tree, task, strings.Split(task.Name(), r.nsSeparator))
}

func (r *Registry) registerTask(tree *taskTree, task Task, parts []string) {
	part := parts[0]
	for _, child := range tree.children {
		if child.name == part {
			if len(parts) == 1 {
				if child.task != nil {
					panic(fmt.Sprintf("duplicate task registered for name %q", task.Name()))
				}
				child.task = task
			} else {
				r.registerTask(child, task, parts[1:])
			}
			return
		}
	}

	child := &taskTree{
		name: part,
	}
	tree.children = append(tree.children, child)

	if len(parts) == 1 {
		child.task = task
	} else {
		r.registerTask(child, task, parts[1:])
	}
}

// Declare a task to be registered.
func (r *Registry) Declare(name string) *Builder {
	tb := build(name)
	r.Register(tb.task)
	return tb
}

func (r *Registry) taskNamespace(t Task) string {
	parts := strings.Split(t.Name(), r.nsSeparator)
	return strings.Join(parts[:len(parts)-1], r.nsSeparator)
}

type taskTree struct {
	name     string
	task     Task
	children []*taskTree
}
