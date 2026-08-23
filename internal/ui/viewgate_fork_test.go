package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

func TestView_UnviewedPaneReturnsLastViewUnchanged(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: true})
	v1 := a.View()

	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	a.messagepane.SetMessages([]messages.MessageItem{
		{TS: "3.0", UserName: "carol", UserID: "U3", Text: "changed content", Timestamp: "1:02 PM"},
	})
	v2 := a.View()
	if v2.Content != v1.Content {
		t.Fatalf("unviewed View() recomputed the screen; want the memoized view back")
	}

	_, _ = a.Update(HerdrTabViewMsg{Viewed: true})
	v3 := a.View()
	if v3.Content == v1.Content {
		t.Fatalf("View() after becoming viewed returned the stale screen; want a fresh render")
	}
}

func TestView_UnviewedPaneRerendersOnResize(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: true})
	v1 := a.View()

	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	a.width = 100
	v2 := a.View()
	if v2.Content == v1.Content {
		t.Fatalf("unviewed View() after a resize returned the old-size screen; want a re-render at the new size")
	}
}

func TestView_OutsideHerdrViewIsNeverGated(t *testing.T) {
	a := newTestAppWithMessages(t)
	v1 := a.View()
	a.messagepane.SetMessages([]messages.MessageItem{
		{TS: "3.0", UserName: "carol", UserID: "U3", Text: "changed content", Timestamp: "1:02 PM"},
	})
	v2 := a.View()
	if v2.Content == v1.Content {
		t.Fatalf("View() outside herdr returned a stale screen; the gate must only fire when a pane is known unviewed")
	}
}
