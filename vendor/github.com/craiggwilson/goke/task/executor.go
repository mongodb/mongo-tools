package task

// Executor executes the body of a task.
type Executor func(*Context) error
