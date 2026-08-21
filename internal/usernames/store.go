// Package usernames owns a workspace's shared userID → display-name map.
package usernames

import (
	"sync"
	"sync/atomic"
)

// Store is the goroutine-safe owner of the map: writers copy-on-write
// under a mutex and publish atomically; readers get the latest
// published map with a pointer load. Maps returned by Current are
// shared and read-only — mutating one is a data race with every other
// reader. All methods are safe on a nil *Store (reads miss, writes
// no-op).
type Store struct {
	mu      sync.Mutex
	current atomic.Pointer[map[string]string]
	version atomic.Uint64
}

func NewStore() *Store {
	s := &Store{}
	m := map[string]string{}
	s.current.Store(&m)
	return s
}

// FromMap returns a store seeded with m's entries.
func FromMap(m map[string]string) *Store {
	s := NewStore()
	s.Apply(m)
	return s
}

// Current returns the latest published map. Read-only; do not mutate.
func (s *Store) Current() map[string]string {
	if s == nil {
		return nil
	}
	return *s.current.Load()
}

func (s *Store) Get(userID string) (string, bool) {
	name, ok := s.Current()[userID]
	return name, ok
}

func (s *Store) Len() int {
	return len(s.Current())
}

// Version increments on every publish (Set and Apply alike). Caches
// keyed on the map's contents compare it to detect change without
// hashing. A no-op write does not bump it.
func (s *Store) Version() uint64 {
	if s == nil {
		return 0
	}
	return s.version.Load()
}

// Set publishes one name. No-op when the store already holds it.
func (s *Store) Set(userID, name string) {
	s.Apply(map[string]string{userID: name})
}

// Apply publishes a batch of names in one copy-and-publish, skipping
// the write entirely when every entry is already current.
func (s *Store) Apply(patch map[string]string) {
	s.apply(patch, true)
}

// Fill is Apply for names learned from a local cache: it publishes
// only entries the store does not already name, so a fill gathered
// during a pass can never overwrite a fresher live-resolved name that
// landed while the pass ran.
func (s *Store) Fill(patch map[string]string) {
	s.apply(patch, false)
}

func (s *Store) apply(patch map[string]string, overwrite bool) {
	if s == nil || len(patch) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := *s.current.Load()
	changed := false
	for id, name := range patch {
		if cur[id] != name && (overwrite || cur[id] == "") {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	next := make(map[string]string, len(cur)+len(patch))
	for id, name := range cur {
		next[id] = name
	}
	for id, name := range patch {
		if overwrite || next[id] == "" {
			next[id] = name
		}
	}
	s.current.Store(&next)
	s.version.Add(1)
}
