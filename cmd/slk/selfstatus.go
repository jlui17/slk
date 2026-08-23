package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammons/slk/internal/ui"
	"github.com/slack-go/slack"
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

// applyBootstrappedStatus is bootstrapPresenceAndDND's write half. A nil
// presence or dnd means that fetch failed and its group is left alone.
// The StatusChangeMsg always carries the store's current winner, so it
// is correct even when every apply was vetoed.
func applyBootstrappedStatus(wctx *WorkspaceContext, program teaSender, tok bootstrapToken, presence *slack.UserPresence, dnd *slack.DNDStatus) {
	if presence != nil {
		wctx.selfStatus.ApplyBootstrapPresence(tok, presence.Presence)
	}

	// Slack's dnd_enabled flag means "the user has a DND schedule
	// configured", NOT "currently in DND". The user is currently in DND
	// only when (a) a manual snooze is active, or (b) the current time
	// falls inside the next scheduled window. The same rule lives in
	// internal/slack/events.go's computeDNDState for the WS event path.
	if dnd != nil {
		now := time.Now().Unix()
		var isDND bool
		var endUnix int64
		switch {
		case dnd.SnoozeEnabled && int64(dnd.SnoozeEndTime) > now:
			isDND = true
			endUnix = int64(dnd.SnoozeEndTime)
		case dnd.Enabled && int64(dnd.NextStartTimestamp) > 0 &&
			int64(dnd.NextStartTimestamp) <= now && now < int64(dnd.NextEndTimestamp):
			isDND = true
			endUnix = int64(dnd.NextEndTimestamp)
		}
		var endTS time.Time
		if endUnix > 0 {
			endTS = time.Unix(endUnix, 0)
		}
		wctx.selfStatus.ApplyBootstrapDND(tok, isDND, endTS)
	}

	if program != nil {
		st := wctx.selfStatus.Snapshot()
		program.Send(ui.StatusChangeMsg{
			TeamID:     wctx.TeamID,
			Presence:   st.Presence,
			DNDEnabled: st.DNDEnabled,
			DNDEndTS:   st.DNDEndTS,
		})
	}
}
