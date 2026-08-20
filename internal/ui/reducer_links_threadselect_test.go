package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// TestThreadPermalinkSelectsLinkedReply pins the end of the thread-link
// journey: completing a pending nav with thread_ts opens the thread
// panel, and when the replies land the cursor sits on the exact linked
// message — not the newest reply that SetThread defaults to.
func TestThreadPermalinkSelectsLinkedReply(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.activeChannelID = "C1"
	a.pendingLinkNav = &pendingLinkNav{
		channelID: "C1",
		messageTS: "300.000003",
		threadTS:  "100.000001",
	}

	_ = a.completePendingLinkNav("C1", true)
	if !a.threadVisible {
		t.Fatal("thread panel should be open")
	}

	_, _ = a.Update(ThreadRepliesLoadedMsg{
		ThreadTS: "100.000001",
		Replies: []messages.MessageItem{
			{TS: "200.000002", Text: "one"},
			{TS: "300.000003", Text: "two"},
			{TS: "400.000004", Text: "three"},
		},
	})
	if sel := a.threadPanel.SelectedReply(); sel == nil || sel.TS != "300.000003" {
		t.Errorf("want linked reply 300.000003 selected, got %+v", sel)
	}

	// A link to the thread parent (thread_ts == message ts) lands on
	// the parent row rather than the newest reply.
	a.CloseThread()
	a.pendingLinkNav = &pendingLinkNav{
		channelID: "C1",
		messageTS: "100.000001",
		threadTS:  "100.000001",
	}
	_ = a.completePendingLinkNav("C1", true)
	_, _ = a.Update(ThreadRepliesLoadedMsg{
		ThreadTS: "100.000001",
		Replies: []messages.MessageItem{
			{TS: "200.000002", Text: "one"},
		},
	})
	if sel := a.threadPanel.SelectedReply(); sel == nil || sel.TS != "100.000001" {
		t.Errorf("parent link: want parent selected, got %+v", sel)
	}
}
