package cli

import "sync"

// recorder is a goroutine-safe accumulator shared between the
// fake-daemon test helpers and the test bodies that assert on them.
// The helpers append incoming requests from their accept-loop
// goroutine while the test reads them from the main goroutine; the
// mutex gives those two a happens-before edge so `go test -race`
// stays clean. Generalises the hand-rolled publishedPayloads pattern
// in terminal_stdout_test.go.
type recorder[T any] struct {
	mu    sync.Mutex
	items []T
}

func (r *recorder[T]) add(v T) {
	r.mu.Lock()
	r.items = append(r.items, v)
	r.mu.Unlock()
}

func (r *recorder[T]) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

func (r *recorder[T]) at(i int) T {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.items[i]
}

// snapshot returns a copy safe to range over or print without holding
// the lock.
func (r *recorder[T]) snapshot() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]T(nil), r.items...)
}
