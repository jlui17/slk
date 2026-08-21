package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
)

// TestPaneStateWriterLastWriteWins pins the reason the writer exists:
// a burst of state changes must persist the newest one, and Close must
// flush a write still pending when the UI loop exits. A quit right
// after closing a thread must not resurrect the thread on relaunch.
func TestPaneStateWriterLastWriteWins(t *testing.T) {
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	w := newPaneStateWriter(db, "p1")
	w.Record(cache.PaneState{WorkspaceID: "T1", ChannelID: "C1"})
	w.Record(cache.PaneState{WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "111.222"})
	w.Record(cache.PaneState{WorkspaceID: "T1", ChannelID: "C1"})
	w.Close()

	got, ok, err := db.GetPaneState("p1")
	if err != nil || !ok {
		t.Fatalf("GetPaneState: ok=%v err=%v", ok, err)
	}
	want := cache.PaneState{WorkspaceID: "T1", ChannelID: "C1"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRestoredChannelFor(t *testing.T) {
	visits := map[string]int64{"CVISIT": 10}
	pane := &cache.PaneState{WorkspaceID: "T1", ChannelID: "CPANE"}

	cases := []struct {
		name   string
		pane   *cache.PaneState
		teamID string
		want   string
	}{
		{"pane's own workspace", pane, "T1", "CPANE"},
		{"other workspace keeps global restore", pane, "T2", "CVISIT"},
		{"no pane state", nil, "T1", "CVISIT"},
		{"pane row without channel", &cache.PaneState{WorkspaceID: "T1"}, "T1", "CVISIT"},
	}
	for _, tc := range cases {
		if got := restoredChannelFor(tc.pane, tc.teamID, visits); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
