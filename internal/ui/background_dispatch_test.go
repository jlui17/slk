package ui

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/messages"
)

// Background-team NewMessageMsg / ThreadMarkedRemoteMsg must not touch
// active-workspace UI state (contract: NewMessageMsg.TeamID).

func TestBackgroundTeamNewMessageIgnored(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	parent := messages.MessageItem{TS: "100.0", Text: "parent", UserID: "U1"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true

	reply := NewMessageMsg{
		TeamID:    "T2",
		ChannelID: "C1",
		Message:   messages.MessageItem{TS: "101.0", ThreadTS: "100.0", Text: "background reply"},
	}
	cmd, handled := reduceSend(a, reply)
	if !handled {
		t.Fatal("background-team NewMessageMsg must still be handled (swallowed)")
	}
	if cmd != nil {
		t.Fatal("background-team NewMessageMsg must not produce a cmd")
	}
	if got := a.threadPanel.ReplyCount(); got != 0 {
		t.Fatalf("background-team reply landed in the thread panel (replies=%d)", got)
	}

	reply.TeamID = "T1"
	_, _ = reduceSend(a, reply)
	if got := a.threadPanel.ReplyCount(); got != 1 {
		t.Fatalf("active-team reply must land in the thread panel (replies=%d)", got)
	}
}

func TestBackgroundTeamMessageDeletedIgnored(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	parent := messages.MessageItem{TS: "100.0", Text: "parent", UserID: "U1"}
	reply := messages.MessageItem{TS: "101.0", ThreadTS: "100.0", Text: "reply", UserID: "U1"}
	a.threadPanel.SetThread(parent, []messages.MessageItem{reply}, "C1", "100.0")
	a.threadVisible = true

	_, handled := reduceSend(a, WSMessageDeletedMsg{TeamID: "T2", ChannelID: "C1", TS: "101.0"})
	if !handled {
		t.Fatal("background-team WSMessageDeletedMsg must still be handled (swallowed)")
	}
	if got := a.threadPanel.ReplyCount(); got != 1 {
		t.Fatalf("background-team deletion removed from the thread panel (replies=%d)", got)
	}

	_, _ = reduceSend(a, WSMessageDeletedMsg{TeamID: "T1", ChannelID: "C1", TS: "101.0"})
	if got := a.threadPanel.ReplyCount(); got != 0 {
		t.Fatalf("active-team deletion must remove from the thread panel (replies=%d)", got)
	}
}

func TestBackgroundTeamThreadMarkIgnored(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	parent := messages.MessageItem{TS: "100.0", Text: "parent", UserID: "U1"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadPanel.SetUnreadBoundary("100.0")
	a.threadVisible = true
	a.threadsView.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "100.0", Unread: true},
	})

	_, handled := reduceThreads(a, ThreadMarkedRemoteMsg{
		TeamID: "T2", ChannelID: "C1", ThreadTS: "100.0", TS: "101.0", Read: true,
	})
	if !handled {
		t.Fatal("background-team ThreadMarkedRemoteMsg must still be handled (swallowed)")
	}
	for _, s := range a.threadsView.Summaries() {
		if s.ThreadTS == "100.0" && !s.Unread {
			t.Fatal("background-team mark flipped the threads-view row")
		}
	}

	_, _ = reduceThreads(a, ThreadMarkedRemoteMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TS: "101.0", Read: true,
	})
	for _, s := range a.threadsView.Summaries() {
		if s.ThreadTS == "100.0" && s.Unread {
			t.Fatal("active-team mark must flip the threads-view row to read")
		}
	}
	// A read mark never clears the open panel's "── new ──" boundary:
	// the user is watching the thread, and the landmark showing which
	// replies were new must survive until they switch threads.
	if got := a.threadPanel.UnreadBoundaryTS(); got != "100.0" {
		t.Fatalf("read mark cleared the open panel's boundary (boundary=%q)", got)
	}
}
