package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// selfStatus is one workspace's self presence/DND state as an immutable
// snapshot. The three fields are consumed together (every StatusChangeMsg
// construction, OnMessage's notification suppression), so they publish
// together: a reader always observes a triple produced by exactly one
// write, never values paired across writes.
type selfStatus struct {
	Presence   string    // "active" or "away"; "" until first fetch
	DNDEnabled bool      // true if either snooze or admin-DND is active
	DNDEndTS   time.Time // unified end timestamp; zero if not in DND
}

// bootstrapToken guards a bootstrap's writes against staleness; the
// ordering rules live on selfStatusStore.
type bootstrapToken struct {
	generation  uint64
	presenceSeq uint64
	dndSeq      uint64
}

// selfStatusStore owns a selfStatus. Writers serialize under mu and
// publish whole snapshots atomically; readers take the latest snapshot
// with a pointer load. The zero value is ready to use. The racing
// goroutines are rostered on WorkspaceContext.selfStatus.
//
// Ordering: WS events always win. Set writes are unconditional and bump
// the field group's seq; a bootstrap's Apply writes are vetoed if the
// generation moved (a newer connection's OnConnect ran BeginBootstrap) or
// the group's seq moved (a WS event wrote while the bootstrap's REST
// snapshot was in flight). Presence and DND are guarded per group so an
// event on one cannot veto the bootstrap's fill of the other.
type selfStatusStore struct {
	mu          sync.Mutex
	current     atomic.Pointer[selfStatus]
	generation  uint64
	presenceSeq uint64
	dndSeq      uint64
}

// Snapshot returns the last published selfStatus, or the zero selfStatus
// before any write. Safe to call from any goroutine.
func (s *selfStatusStore) Snapshot() selfStatus {
	if cur := s.current.Load(); cur != nil {
		return *cur
	}
	return selfStatus{}
}

// SetPresence records a WS presence event and returns the snapshot it
// published, so the caller can send exactly what it wrote.
func (s *selfStatusStore) SetPresence(presence string) selfStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.presenceSeq++
	next := s.Snapshot()
	next.Presence = presence
	s.current.Store(&next)
	return next
}

// SetDND records a WS DND event and returns the snapshot it published.
func (s *selfStatusStore) SetDND(enabled bool, endTS time.Time) selfStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dndSeq++
	next := s.Snapshot()
	next.DNDEnabled = enabled
	next.DNDEndTS = endTS
	s.current.Store(&next)
	return next
}

// BeginBootstrap opens a new connection generation and returns the token
// the connection's bootstrap must pass to its Apply calls. Calling it
// invalidates every earlier token.
func (s *selfStatusStore) BeginBootstrap() bootstrapToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	return bootstrapToken{
		generation:  s.generation,
		presenceSeq: s.presenceSeq,
		dndSeq:      s.dndSeq,
	}
}

// ApplyBootstrapPresence fills the presence from a bootstrap fetch.
// Vetoed (no-op, false) if the token is stale for the presence group.
func (s *selfStatusStore) ApplyBootstrapPresence(tok bootstrapToken, presence string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation != tok.generation || s.presenceSeq != tok.presenceSeq {
		return false
	}
	next := s.Snapshot()
	next.Presence = presence
	s.current.Store(&next)
	return true
}

// ApplyBootstrapDND fills the DND pair from a bootstrap fetch. Vetoed
// (no-op, false) if the token is stale for the DND group.
func (s *selfStatusStore) ApplyBootstrapDND(tok bootstrapToken, enabled bool, endTS time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation != tok.generation || s.dndSeq != tok.dndSeq {
		return false
	}
	next := s.Snapshot()
	next.DNDEnabled = enabled
	next.DNDEndTS = endTS
	s.current.Store(&next)
	return true
}
