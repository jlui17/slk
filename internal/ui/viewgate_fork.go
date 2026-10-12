package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/statusbar"
)

// viewMemo is the last fully-rendered view, kept so an unviewed pane can
// answer View() without rebuilding panels. Sized so a resize while
// unviewed falls through to a real render instead of returning a screen
// of the wrong dimensions, and keyed by the connection state it shows so
// a connect, drop or redial while unviewed draws one real frame: the
// connection segment is what a glance at a parked pane is for.
type viewMemo struct {
	view  tea.View
	w, h  int
	conn  statusbar.ConnectionState
	valid bool
}

// unviewedLastView returns the memoized view while the pane is parked in
// an unviewed herdr tab. WS events keep mutating model state through
// Update, but nothing the pane renders can be seen, so panel rebuilds
// are deferred until the tab is viewed again: the version bumps those
// events made are still pending, and the first viewed View() renders
// them all at once. Returning the identical view (WindowTitle included)
// also lets Bubble Tea's renderer skip the flush entirely.
//
// A settling screen is exempt: while it is up, the gate would freeze
// the pty on a frame that is about to change by itself, and herdr
// surfaces that stale screen on paths that emit no focus event
// (returning to the terminal window), so an unviewed launch reads as a
// hung "Connecting" forever. These frames are short-lived; render them
// for real.
func (a *App) unviewedLastView() (tea.View, bool) {
	if a.screenSettling() || a.PaneViewed() || !a.lastView.valid || a.lastView.w != a.width || a.lastView.h != a.height || a.lastView.conn != a.statusbar.ConnectionState() {
		return tea.View{}, false
	}
	return a.lastView.view, true
}

// rememberLastView stores the view a completed View() pass produced;
// every full render refreshes the memo the unviewed gate serves.
// Settling frames are not stored: the first View() after the screen
// settles must render the settled UI, not hand an unviewed pane the
// loading or connecting screen back.
func (a *App) rememberLastView(v tea.View) tea.View {
	if a.screenSettling() {
		a.lastView = viewMemo{}
		return v
	}
	a.lastView = viewMemo{view: v, w: a.width, h: a.height, conn: a.statusbar.ConnectionState(), valid: true}
	return v
}

// screenSettling reports whether the screen is mid-startup or mid-sync:
// the bootstrap overlay is up, or the "cached · syncing" pill is.
func (a *App) screenSettling() bool {
	return a.bootstrap.IsLoading() || a.statusbar.Syncing()
}
