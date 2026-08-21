package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/usernames"
)

// An inactive workspace's thread events must reach the UI tagged with
// the workspace (contract: ui.NewMessageMsg.TeamID); its top-level
// messages must not dispatch at all.

func sentOfType[T tea.Msg](c *captureSender) []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []T
	for _, m := range c.sent {
		if v, ok := m.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

func TestOnMessageInactiveWorkspaceDispatchesTagged(t *testing.T) {
	sender := &captureSender{}
	h := &rtmEventHandler{
		program:     sender,
		workspaceID: "T2",
		userNames:   usernames.FromMap(map[string]string{"U1": "user one"}),
		isActive:    func() bool { return false },
	}
	h.OnMessage("C1", "U1", "101.0", "a thread reply", "100.0", "", false, nil, slack.Blocks{}, nil, "", "")

	msgs := sentOfType[ui.NewMessageMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 NewMessageMsg from an inactive workspace, got %d", len(msgs))
	}
	m := msgs[0]
	if m.TeamID != "T2" || m.ChannelID != "C1" || m.Message.TS != "101.0" || m.Message.ThreadTS != "100.0" {
		t.Fatalf("NewMessageMsg mis-tagged: %+v", m)
	}
	if rs := sentOfType[ui.ReadStateChangedMsg](sender); len(rs) != 1 {
		t.Fatalf("inactive path must still fire ReadStateChangedMsg for the rail dot, got %d", len(rs))
	}
}

func TestOnMessageInactiveTopLevelNotDispatched(t *testing.T) {
	sender := &captureSender{}
	h := &rtmEventHandler{
		program:     sender,
		workspaceID: "T2",
		userNames:   usernames.FromMap(map[string]string{"U1": "user one"}),
		isActive:    func() bool { return false },
	}
	h.OnMessage("C1", "U1", "101.0", "a top-level message", "", "", false, nil, slack.Blocks{}, nil, "", "")

	if msgs := sentOfType[ui.NewMessageMsg](sender); len(msgs) != 0 {
		t.Fatalf("background top-level messages must not dispatch NewMessageMsg (no consumer; per-message render cost), got %d", len(msgs))
	}
	if rs := sentOfType[ui.ReadStateChangedMsg](sender); len(rs) != 1 {
		t.Fatalf("inactive path must fire ReadStateChangedMsg, got %d", len(rs))
	}
}

func TestOnMessageDeletedInactiveWorkspaceDispatchesTagged(t *testing.T) {
	sender := &captureSender{}
	h := &rtmEventHandler{
		program:     sender,
		workspaceID: "T2",
		db:          newTestDB(t),
		isActive:    func() bool { return false },
	}
	h.OnMessageDeleted("C1", "101.0")

	msgs := sentOfType[ui.WSMessageDeletedMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 WSMessageDeletedMsg from an inactive workspace, got %d", len(msgs))
	}
	if m := msgs[0]; m.TeamID != "T2" || m.ChannelID != "C1" || m.TS != "101.0" {
		t.Fatalf("WSMessageDeletedMsg mis-tagged: %+v", m)
	}
}

func TestOnThreadMarkedInactiveWorkspaceDispatchesTagged(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T2", Name: "T2"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	// Seed the thread so the read decision is derivable: parent + one
	// reply, with the mark landing at the reply (read to the end).
	for _, m := range []cache.Message{
		{TS: "100.0", ChannelID: "C1", WorkspaceID: "T2", UserID: "U1", Text: "parent"},
		{TS: "101.0", ChannelID: "C1", WorkspaceID: "T2", UserID: "U2", Text: "reply", ThreadTS: "100.0"},
	} {
		if err := db.UpsertMessage(m); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	sender := &captureSender{}
	h := &rtmEventHandler{
		db:          db,
		program:     sender,
		workspaceID: "T2",
		isActive:    func() bool { return false },
	}
	h.OnThreadMarked("C1", "100.0", "101.0", true)

	msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 ThreadMarkedRemoteMsg from an inactive workspace, got %d", len(msgs))
	}
	m := msgs[0]
	if m.TeamID != "T2" || m.ChannelID != "C1" || m.ThreadTS != "100.0" || m.TS != "101.0" || !m.Read {
		t.Fatalf("ThreadMarkedRemoteMsg mis-tagged: %+v", m)
	}
}
