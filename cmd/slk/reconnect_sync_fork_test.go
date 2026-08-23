package main

import (
	"context"
	"testing"
	"time"

	"github.com/gammons/slk/internal/ui"
)

// TestDedupeGateReset pins the manual-reload contract: reset clears
// the window so the next pass runs immediately.
func TestDedupeGateReset(t *testing.T) {
	g := dedupeGate{window: 30 * time.Second}
	now := time.Now()
	if !g.tryStart(now) {
		t.Fatal("first tryStart must succeed")
	}
	if g.tryStart(now.Add(time.Second)) {
		t.Fatal("tryStart inside the window must be suppressed")
	}
	g.reset()
	if !g.tryStart(now.Add(2 * time.Second)) {
		t.Fatal("tryStart after reset must succeed")
	}
}

// Ten instances share one cache.db. Zeroing synced_at there on
// reconnect poisoned every sibling instance's freshness tiers: one
// laptop wake made all ten refetch every channel they opened. The
// reconnect pass must leave the shared DB alone and instead send this
// instance's UI a watermark that demotes provably-fresh reads locally.
func TestReconnect_SendsWatermarkAndLeavesSharedSyncedAtAlone(t *testing.T) {
	db := newTestDB(t)
	ids := seedWorkspaceChannels(t, db, 3)

	sender := &captureSender{}
	start := time.Now()
	r := &reconnectSync{
		client: &fakeCounts{}, db: db, workspaceID: "T1", program: sender,
		activeChannel:  func() string { return ids[1] },
		refreshChannel: func(context.Context, string) {},
	}
	if err := r.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, id := range ids {
		if got := db.GetChannelSyncedAt(id); got != 1700000000 {
			t.Errorf("%s synced_at = %d; want 1700000000 untouched — the shared DB is other instances' freshness state", id, got)
		}
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	var marks []ui.CacheStaleBeforeMsg
	for _, m := range sender.sent {
		if wm, ok := m.(ui.CacheStaleBeforeMsg); ok {
			marks = append(marks, wm)
		}
	}
	if len(marks) != 1 {
		t.Fatalf("sent %d CacheStaleBeforeMsg; want exactly 1", len(marks))
	}
	wm := marks[0]
	if wm.WorkspaceID != "T1" {
		t.Errorf("watermark workspace = %q; want T1", wm.WorkspaceID)
	}
	if wm.Before.Before(start) || wm.Before.After(time.Now()) {
		t.Errorf("watermark time %v outside the run window [%v, now]", wm.Before, start)
	}
}
