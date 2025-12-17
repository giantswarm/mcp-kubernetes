package k8s

import "sync"

// lazyValue provides thread-safe lazy initialization for any type.
// It uses double-check locking to minimize lock contention while ensuring
// thread-safe initialization.
type lazyValue[T any] struct {
	mu    sync.RWMutex
	value T
	set   bool
}

// Get returns the cached value if set, otherwise calls initFn to create it.
// This method is thread-safe and uses double-check locking for efficiency.
//
// The initFn is called at most once, even with concurrent access.
// If initFn returns an error, the value is not cached and subsequent calls
// will retry initialization.
func (l *lazyValue[T]) Get(initFn func() (T, error)) (T, error) {
	// Fast path: check if already initialized
	l.mu.RLock()
	if l.set {
		v := l.value
		l.mu.RUnlock()
		return v, nil
	}
	l.mu.RUnlock()

	// Slow path: acquire write lock and initialize
	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after acquiring write lock
	if l.set {
		return l.value, nil
	}

	v, err := initFn()
	if err != nil {
		var zero T
		return zero, err
	}

	l.value = v
	l.set = true
	return v, nil
}

// IsSet returns true if the value has been initialized.
func (l *lazyValue[T]) IsSet() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.set
}
