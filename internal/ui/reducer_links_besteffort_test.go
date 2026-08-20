package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// TestPendingLinkNavSurvivesBestEffortSelect pins the tier-2 permalink
// path: when the target is already in the cached render, the
// best-effort completion selects it but must keep the nav armed —
// the in-flight fetch's MessagesLoadedMsg replaces the buffer (which
// clears the selection and snaps to the bottom), and only the
// authoritative completion that follows may retire the nav, after
// re-selecting the target on the fresh buffer.
func TestPendingLinkNavSurvivesBestEffortSelect(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.activeChannelID = "C1"
	buf := []messages.MessageItem{
		{TS: "100.000001", Text: "one"},
		{TS: "200.000002", Text: "two"},
		{TS: "300.000003", Text: "three"},
	}
	a.messagepane.SetMessages(buf)
	a.pendingLinkNav = &pendingLinkNav{channelID: "C1", messageTS: "200.000002"}

	// Best-effort pass (cache render, fetch in flight): selects the
	// target but keeps the nav.
	if cmd := a.completePendingLinkNav("C1", false); cmd != nil {
		t.Error("best-effort select success should return no cmd")
	}
	if sel, _ := a.messagepane.SelectedMessage(); sel.TS != "200.000002" {
		t.Errorf("best-effort: want target selected, got %q", sel.TS)
	}
	if a.pendingLinkNav == nil {
		t.Fatal("best-effort select success cleared pendingLinkNav; authoritative replace would lose the jump")
	}

	// The verify fetch lands: buffer replaced, selection snaps to the
	// bottom (SetMessages semantics), then the authoritative pass
	// re-selects the target and retires the nav.
	a.messagepane.SetMessages(buf)
	if cmd := a.completePendingLinkNav("C1", true); cmd != nil {
		t.Error("authoritative select success should return no cmd")
	}
	if sel, _ := a.messagepane.SelectedMessage(); sel.TS != "200.000002" {
		t.Errorf("authoritative: want target re-selected, got %q", sel.TS)
	}
	if a.pendingLinkNav != nil {
		t.Errorf("authoritative success must retire the nav, got %+v", a.pendingLinkNav)
	}
}
