package ui

import tea "charm.land/bubbletea/v2"

// viewMemo is the last fully-rendered view, kept so an unviewed pane can
// answer View() without rebuilding panels. Sized so a resize while
// unviewed falls through to a real render instead of returning a screen
// of the wrong dimensions.
type viewMemo struct {
	view  tea.View
	w, h  int
	valid bool
}

// unviewedLastView returns the memoized view while the pane is parked in
// an unviewed herdr tab. WS events keep mutating model state through
// Update, but nothing the pane renders can be seen, so panel rebuilds
// are deferred until the tab is viewed again: the version bumps those
// events made are still pending, and the first viewed View() renders
// them all at once. Returning the identical view (WindowTitle included)
// also lets Bubble Tea's renderer skip the flush entirely.
func (a *App) unviewedLastView() (tea.View, bool) {
	if a.PaneViewed() || !a.lastView.valid || a.lastView.w != a.width || a.lastView.h != a.height {
		return tea.View{}, false
	}
	return a.lastView.view, true
}

// rememberLastView stores the view a completed View() pass produced;
// every full render refreshes the memo the unviewed gate serves.
func (a *App) rememberLastView(v tea.View) tea.View {
	a.lastView = viewMemo{view: v, w: a.width, h: a.height, valid: true}
	return v
}
