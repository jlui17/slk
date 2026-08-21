// Package sharedmap owns a map read and written from multiple
// goroutines: writers copy-on-write under a mutex and publish
// atomically; readers get the latest published map with a pointer
// load. usernames.Store is deliberately not a client: it needs
// fill-versus-update semantics and a version counter that the maps
// stored here don't.
package sharedmap

import (
	"sync"
	"sync/atomic"
)

// Map is the goroutine-safe owner of a map[K]V. Maps returned by
// Current are shared and read-only — mutating one is a data race with
// every other reader. All methods are safe on a nil *Map (reads miss,
// writes no-op).
type Map[K comparable, V any] struct {
	mu      sync.Mutex
	current atomic.Pointer[map[K]V]
}

func New[K comparable, V any]() *Map[K, V] {
	m := &Map[K, V]{}
	empty := map[K]V{}
	m.current.Store(&empty)
	return m
}

// FromMap returns a map seeded with src's entries, copied — the caller
// keeps ownership of src.
func FromMap[K comparable, V any](src map[K]V) *Map[K, V] {
	m := New[K, V]()
	if len(src) == 0 {
		return m
	}
	next := make(map[K]V, len(src))
	for k, v := range src {
		next[k] = v
	}
	m.current.Store(&next)
	return m
}

// Current returns the latest published map. Read-only; do not mutate.
func (m *Map[K, V]) Current() map[K]V {
	if m == nil {
		return nil
	}
	return *m.current.Load()
}

func (m *Map[K, V]) Get(k K) (V, bool) {
	v, ok := m.Current()[k]
	return v, ok
}

// Set publishes one entry. Every call copies and republishes the whole
// map — there is no same-value short-circuit — so callers with large
// maps and hot write paths should not use this type.
func (m *Map[K, V]) Set(k K, v V) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := *m.current.Load()
	next := make(map[K]V, len(cur)+1)
	for key, val := range cur {
		next[key] = val
	}
	next[k] = v
	m.current.Store(&next)
}
