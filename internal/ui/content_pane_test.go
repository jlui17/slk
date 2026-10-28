package ui

import "testing"

func TestFocusedContentPane(t *testing.T) {
	a := newTestApp(t, withMessages(testMessageItems(3)...), withActiveChannel("C1"))
	a.focusedPanel = PanelMessages
	if p, ch, ok := a.focusedContentPane(); !ok || p != contentPane(a.messagepane) || ch != "C1" {
		t.Fatalf("messages: %v %q %v", p, ch, ok)
	}
	a.threadPanel.SetThread(testMessageItems(1)[0], nil, "C9", "1.0")
	a.focusedPanel = PanelThread
	if p, ch, ok := a.focusedContentPane(); !ok || p != contentPane(a.threadPanel) || ch != "C9" {
		t.Fatalf("thread: %v %q %v", p, ch, ok)
	}
	a.focusedPanel = PanelSidebar
	if _, _, ok := a.focusedContentPane(); ok {
		t.Fatal("the sidebar is not a content pane")
	}
}

func TestContentPaneFor(t *testing.T) {
	a := newTestApp(t)
	if a.contentPaneFor(PanelMessages) != contentPane(a.messagepane) {
		t.Fatal("PanelMessages is the channel pane")
	}
	if a.contentPaneFor(PanelThread) != contentPane(a.threadPanel) {
		t.Fatal("PanelThread is the thread pane")
	}
	if a.contentPaneFor(PanelSidebar) != nil {
		t.Fatal("the sidebar is not a content pane")
	}
}
