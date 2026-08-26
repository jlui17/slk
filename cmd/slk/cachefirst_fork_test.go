package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
)

func seedCacheFirstDB(t *testing.T, db *cache.DB) {
	t.Helper()
	chans := []cache.Channel{
		{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true},
		{ID: "C2", WorkspaceID: "T1", Name: "random", Type: "channel", IsMember: true},
		{ID: "CLEFT", WorkspaceID: "T1", Name: "left-behind", Type: "channel", IsMember: false},
		{ID: "D1", WorkspaceID: "T1", Name: "", Type: "dm", IsMember: true},
		{ID: "G1", WorkspaceID: "T1", Name: "mpdm-alice--bob-1", Type: "group_dm", IsMember: true},
	}
	for _, ch := range chans {
		if err := db.UpsertChannel(ch); err != nil {
			t.Fatalf("UpsertChannel %s: %v", ch.ID, err)
		}
	}
	if err := db.UpsertUser(cache.User{ID: "UA", WorkspaceID: "T1", Name: "alice", DisplayName: "Alice A"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := db.RecordChannelVisit("T1", "C2"); err != nil {
		t.Fatalf("RecordChannelVisit: %v", err)
	}
}

func TestCachedWorkspaceMsgFromSeededDB(t *testing.T) {
	db := newCacheForTest(t)
	seedCacheFirstDB(t, db)
	l := newCacheFirstLoader(db, config.Config{}, "15:04", newWorkspaceRouter())

	msg, ok := l.cachedWorkspaceMsg("T1", "Acme", nil)
	if !ok {
		t.Fatal("seeded cache produced no provisional workspace")
	}
	byID := map[string]string{}
	for _, it := range msg.Channels {
		byID[it.ID] = it.Name
	}
	if byID["C1"] != "general" || byID["C2"] != "random" {
		t.Errorf("channel names = %v, want general/random from cache", byID)
	}
	if _, ok := byID["CLEFT"]; ok {
		t.Error("non-member channel leaked into the provisional sidebar")
	}
	if _, ok := byID["D1"]; ok {
		t.Error("nameless DM row leaked into the provisional sidebar")
	}
	if got := byID["G1"]; !strings.Contains(got, "Alice A") {
		t.Errorf("group DM name = %q, want handles expanded via cached users", got)
	}
	if msg.LastChannelID != "C2" {
		t.Errorf("LastChannelID = %q, want the visited C2", msg.LastChannelID)
	}
	var finderC2 int64
	for _, it := range msg.FinderItems {
		if it.ID == "C2" {
			finderC2 = it.LastVisited
		}
	}
	if finderC2 == 0 {
		t.Error("finder items missing cached last-visited recency")
	}
}

func TestCachedWorkspaceMsgPaneRestoreWins(t *testing.T) {
	db := newCacheForTest(t)
	seedCacheFirstDB(t, db)
	l := newCacheFirstLoader(db, config.Config{}, "15:04", newWorkspaceRouter())

	msg, ok := l.cachedWorkspaceMsg("T1", "Acme", &cache.PaneState{WorkspaceID: "T1", ChannelID: "C1"})
	if !ok || msg.LastChannelID != "C1" {
		t.Errorf("pane-state channel must win over visit recency: ok=%v last=%q", ok, msg.LastChannelID)
	}
}

func TestSendCachedWorkspaceClaims(t *testing.T) {
	db := newCacheForTest(t)
	seedCacheFirstDB(t, db)
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T2", Name: "Empty"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	tokens := []config.OrderedToken{
		{Token: slackclient.Token{TeamID: "T2", TeamName: "Empty"}},
		{Token: slackclient.Token{TeamID: "T1", TeamName: "Acme"}},
	}

	send := func(got *[]ui.WorkspaceCachedMsg) func(tea.Msg) {
		return func(m tea.Msg) {
			if wm, ok := m.(ui.WorkspaceCachedMsg); ok {
				*got = append(*got, wm)
			}
		}
	}

	// No default: the first workspace with a non-empty cache claims.
	l := newCacheFirstLoader(db, config.Config{}, "15:04", newWorkspaceRouter())
	var got []ui.WorkspaceCachedMsg
	l.SendCachedWorkspace(tokens, "", nil, send(&got))
	if len(got) != 1 || got[0].TeamID != "T1" {
		t.Fatalf("no-default claim = %v, want one msg for T1", got)
	}
	if l.claimedTeam() != "T1" {
		t.Errorf("claimedTeam = %q, want T1", l.claimedTeam())
	}

	// Default set: only that workspace may claim.
	l = newCacheFirstLoader(db, config.Config{}, "15:04", newWorkspaceRouter())
	got = nil
	l.SendCachedWorkspace(tokens, "T1", nil, send(&got))
	if len(got) != 1 || got[0].TeamID != "T1" {
		t.Fatalf("default-team claim = %v, want one msg for T1", got)
	}

	// Default set but its cache is empty: nothing may steal active.
	l = newCacheFirstLoader(db, config.Config{}, "15:04", newWorkspaceRouter())
	got = nil
	l.SendCachedWorkspace(tokens, "T2", nil, send(&got))
	if len(got) != 0 {
		t.Fatalf("empty-cache default sent %v, want nothing (overlay stays)", got)
	}
	if l.claimedTeam() != "" {
		t.Errorf("claimedTeam = %q, want none", l.claimedTeam())
	}
}

func TestCacheFirstReadFallbacks(t *testing.T) {
	db := newCacheForTest(t)
	seedCacheFirstDB(t, db)
	if err := db.UpsertMessage(cache.Message{
		TS:          "1700000001.000000",
		ChannelID:   "C2",
		WorkspaceID: "T1",
		UserID:      "UA",
		Text:        "cached hello",
		CreatedAt:   1700000001,
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	if err := db.UpdateChannelReadState("C2", "1700000000.000000", true); err != nil {
		t.Fatalf("UpdateChannelReadState: %v", err)
	}
	l := newCacheFirstLoader(db, config.Config{}, "15:04", newWorkspaceRouter())

	// Unclaimed: the fallbacks serve nothing (no workspace owns the UI).
	if l.readCache("C2") != nil || l.readState() != nil {
		t.Error("fallbacks must be inert before a provisional claim")
	}

	tokens := []config.OrderedToken{{Token: slackclient.Token{TeamID: "T1", TeamName: "Acme"}}}
	l.SendCachedWorkspace(tokens, "", nil, func(tea.Msg) {})

	msgs := l.readCache("C2")
	if len(msgs) != 1 || msgs[0].Text != "cached hello" {
		t.Errorf("readCache = %v, want the cached message", msgs)
	}
	if msgs[0].UserName != "Alice A" {
		t.Errorf("author = %q, want name resolved from the cached users table", msgs[0].UserName)
	}
	state := l.readState()
	if st, ok := state["C2"]; !ok || !st.HasUnread || st.LastReadTS != "1700000000.000000" {
		t.Errorf("readState[C2] = %+v, want the persisted read state", state["C2"])
	}
}
