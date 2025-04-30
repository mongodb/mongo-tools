package util

import "sync"

// DataGuard encapsulates a value with a mutex to ensure that anything that
// accesses it does so in a race-safe way.
type DataGuard[T any] struct {
	mutex sync.RWMutex
	value T
}

// NewDataGuard returns a new DataGuard that wraps the given value.
func NewDataGuard[T any](val T) *DataGuard[T] {
	return &DataGuard[T]{
		value: val,
	}
}

// Load runs the given callback, passing it the DataGuard’s stored value.
func (l *DataGuard[T]) Load(cb func(T)) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	cb(l.value)
}

// GetValue is like Load, but returns the value directly. This is useful if you just need the
// current value and do not care if it changes after you retrieve it.
func (l *DataGuard[T]) GetValue() T {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	return l.value
}

// Store is like Load but will replace the DataGuard’s stored value with the
// callback’s return.
func (l *DataGuard[T]) Store(cb func(T) T) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.value = cb(l.value)
}
