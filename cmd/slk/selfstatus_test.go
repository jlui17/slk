package main

import (
	"testing"
	"time"
)

// A never-written store must serve the zero selfStatus, not dereference
// the nil pointer inside it. This path is live on every workspace between
// construction and the first bootstrap apply.
func TestSelfStatusZeroValueSnapshot(t *testing.T) {
	var s selfStatusStore
	st := s.Snapshot()
	if st.Presence != "" || st.DNDEnabled || !st.DNDEndTS.IsZero() {
		t.Fatalf("zero-value Snapshot = %+v, want zero selfStatus", st)
	}
}

func TestSelfStatusBootstrapFillApplies(t *testing.T) {
	var s selfStatusStore
	tok := s.BeginBootstrap()
	end := time.Unix(1755800000, 0)
	if !s.ApplyBootstrapPresence(tok, "active") {
		t.Fatal("presence fill vetoed with no intervening writes")
	}
	if !s.ApplyBootstrapDND(tok, true, end) {
		t.Fatal("DND fill vetoed with no intervening writes")
	}
	st := s.Snapshot()
	if st.Presence != "active" || !st.DNDEnabled || !st.DNDEndTS.Equal(end) {
		t.Fatalf("Snapshot = %+v, want the filled triple", st)
	}
}

// A WS event that lands while the bootstrap's REST snapshot is in flight
// must win: the bootstrap's apply for that group is vetoed, and only that
// group — the other group's fill still applies.
func TestSelfStatusEventVetoesBootstrapPerGroup(t *testing.T) {
	var s selfStatusStore
	tok := s.BeginBootstrap()
	s.SetDND(true, time.Unix(1755800000, 0))
	if s.ApplyBootstrapDND(tok, false, time.Time{}) {
		t.Fatal("stale bootstrap DND overwrote a newer WS event")
	}
	if !s.ApplyBootstrapPresence(tok, "active") {
		t.Fatal("DND event vetoed the independent presence fill")
	}
	st := s.Snapshot()
	if !st.DNDEnabled || st.Presence != "active" {
		t.Fatalf("Snapshot = %+v, want event DND + bootstrap presence", st)
	}

	var s2 selfStatusStore
	tok2 := s2.BeginBootstrap()
	s2.SetPresence("away")
	if s2.ApplyBootstrapPresence(tok2, "active") {
		t.Fatal("stale bootstrap presence overwrote a newer WS event")
	}
	if !s2.ApplyBootstrapDND(tok2, true, time.Unix(1755800000, 0)) {
		t.Fatal("presence event vetoed the independent DND fill")
	}
}

// A reconnect (BeginBootstrap) invalidates every earlier token outright:
// connection N's slow bootstrap can write nothing once connection N+1's
// OnConnect has run, even though no WS event intervened.
func TestSelfStatusReconnectVetoesOldGeneration(t *testing.T) {
	var s selfStatusStore
	tokN := s.BeginBootstrap()
	tokN1 := s.BeginBootstrap()
	if s.ApplyBootstrapPresence(tokN, "away") {
		t.Fatal("old-generation presence apply accepted")
	}
	if s.ApplyBootstrapDND(tokN, true, time.Unix(1755800000, 0)) {
		t.Fatal("old-generation DND apply accepted")
	}
	if !s.ApplyBootstrapPresence(tokN1, "active") {
		t.Fatal("current-generation presence apply vetoed")
	}
	if st := s.Snapshot(); st.Presence != "active" {
		t.Fatalf("Presence = %q, want the current generation's fill", st.Presence)
	}
}
