package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// findThreadReactionHit walks the rendered thread pane's content area
// for a coordinate where threadPanel.HitTestReaction reports wantEmoji,
// and returns it in app-level coordinates for a tea.MouseClickMsg.
func findThreadReactionHit(t *testing.T, a *App, wantEmoji string) (x, y int) {
	t.Helper()
	chrome := a.threadPanel.ChromeHeight()
	maxPaneY := a.height - 2
	maxPaneX := a.layout.threadEnd - a.layout.msgEnd - 2
	for paneY := chrome; paneY < maxPaneY; paneY++ {
		for paneX := 0; paneX < maxPaneX; paneX++ {
			if _, e, ok := a.threadPanel.HitTestReaction(paneY-chrome, paneX); ok && e == wantEmoji {
				return a.layout.msgEnd + 1 + paneX, paneY + 1
			}
		}
	}
	t.Fatalf("no %q reaction hit found in thread pane after render", wantEmoji)
	return 0, 0
}

// TestApp_ClickOnThreadReactionPillTogglesReaction pins the thread
// half of the pill click: a left click on a reply's pill, through the
// real Update/reduceMouseClick path, toggles that reaction on that
// reply in the thread's channel (not the active channel), and does not
// begin a drag selection.
func TestApp_ClickOnThreadReactionPillTogglesReaction(t *testing.T) {
	a := NewApp()
	a.width = 200 // wide enough that the thread sits beside the channel, not stacked
	a.height = 30
	a.activeChannelID = "C-active"
	a.currentUserID = "U-self"
	a.threadPanel.SetThread(
		messages.MessageItem{TS: "1700000000.000000", UserID: "U1", UserName: "alice", Text: "parent", Timestamp: "10:00 AM"},
		[]messages.MessageItem{
			{
				TS: "1700000001.000000", UserID: "U2", UserName: "bob", Text: "first", Timestamp: "10:01 AM",
				Reactions: []messages.ReactionItem{{Emoji: "eyes", Count: 1, HasReacted: true}},
			},
			{
				TS: "1700000002.000000", UserID: "U3", UserName: "carol", Text: "second", Timestamp: "10:02 AM",
				Reactions: []messages.ReactionItem{{Emoji: "thumbsup", Count: 1, HasReacted: false}},
			},
		},
		"C-thread", "1700000000.000000")
	a.threadVisible = true
	_ = a.View()
	if a.layout.threadEnd <= a.layout.msgEnd {
		t.Fatalf("precondition: thread pane not drawn (bands %+v)", *a.layout)
	}

	type call struct{ channelID, ts, emoji string }
	var added, removed []call
	a.SetReactionService(core.NewReactionService(
		func(channelID ids.ChannelID, ts ids.MessageTS, emoji string) error {
			added = append(added, call{string(channelID), string(ts), emoji})
			return nil
		},
		func(channelID ids.ChannelID, ts ids.MessageTS, emoji string) error {
			removed = append(removed, call{string(channelID), string(ts), emoji})
			return nil
		},
		nil, nil,
	))

	x, y := findThreadReactionHit(t, a, "thumbsup")
	_, cmd := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd for the thread reaction click; got nil")
	}
	_ = drainBatch(cmd)

	if a.focusedPanel != PanelThread {
		t.Errorf("focusedPanel = %v, want PanelThread", a.focusedPanel)
	}
	if len(added) != 1 || len(removed) != 0 {
		t.Fatalf("want 1 add and 0 removes, got added=%v removed=%v", added, removed)
	}
	want := call{"C-thread", "1700000002.000000", "thumbsup"}
	if added[0] != want {
		t.Errorf("add call = %+v, want %+v", added[0], want)
	}
	if a.threadPanel.HasSelection() {
		t.Error("pill click must not begin a drag selection in the thread pane")
	}

	// The already-reacted pill on the other reply removes instead.
	x, y = findThreadReactionHit(t, a, "eyes")
	_, cmd = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd for the second thread reaction click; got nil")
	}
	_ = drainBatch(cmd)
	if len(removed) != 1 {
		t.Fatalf("want 1 remove, got removed=%v", removed)
	}
	if want := (call{"C-thread", "1700000001.000000", "eyes"}); removed[0] != want {
		t.Errorf("remove call = %+v, want %+v", removed[0], want)
	}
}
