package ui

import (
	"testing"

	"github.com/gammons/slk/internal/core"
)

// TestRenderSidebar_ReadStateRefreshMatchesFreshRender re-reads the
// sidebar's read state the way a message in another channel does,
// with and without a change to that channel's row, and checks each
// frame against a fresh render. A refresh that changes no row must
// leave the frame as it was; one that does must show it.
func TestRenderSidebar_ReadStateRefreshMatchesFreshRender(t *testing.T) {
	readState := map[string]core.ReadState{}
	a := newTestApp(t, withWindowSize(200, 40), withChannels(benchChannels()...), withActiveChannel("C1"))
	a.setReadStateReaderForTest(func() map[string]core.ReadState { return readState })

	fresh := func() string {
		a.renderCache.sidebar = panelCache{}
		a.renderCache.sidebarView = ""
		return a.View().Content
	}

	steps := []struct {
		name    string
		do      func()
		changes bool
	}{
		{"refresh, nothing new", func() {}, false},
		{"channel turns unread", func() { readState["C7"] = core.ReadState{HasUnread: true} }, true},
		{"refresh, still unread", func() {}, false},
		{"mention badge", func() { readState["C7"] = core.ReadState{HasUnread: true, MentionCount: 2} }, true},
		{"channel read", func() { delete(readState, "C7") }, true},
	}

	prev := a.View().Content
	for _, s := range steps {
		s.do()
		a.notifyReadStateChanged()
		got := a.View().Content
		if changed := got != prev; changed != s.changes {
			t.Fatalf("%s: frame changed = %v, want %v", s.name, changed, s.changes)
		}
		if want := fresh(); got != want {
			t.Fatalf("%s: cached frame differs from a fresh render", s.name)
		}
		prev = got
	}
}
