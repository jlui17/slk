package ui

// focusPeerWhileZoomed is the whole focus cycle while the thread is
// zoomed: the messages pane isn't drawn, so it's Sidebar <-> Thread,
// or Thread alone when the sidebar is hidden too. A two-stop cycle is
// symmetric, so FocusNext and FocusPrev share it.
func (a *App) focusPeerWhileZoomed() Panel {
	if a.sidebarVisible && a.focusedPanel == PanelThread {
		return PanelSidebar
	}
	return PanelThread
}

// ToggleThreadFullscreen zooms the open thread over the whole messages
// region, or restores the side-by-side layout. No-op with no thread
// open. Zooming pulls focus off the messages pane, which stops being
// drawn.
func (a *App) ToggleThreadFullscreen() {
	if !a.threadVisible {
		return
	}
	a.clearSelections()
	a.threadFullscreen = !a.threadFullscreen
	if a.threadFullscreen && a.focusedPanel == PanelMessages {
		a.focusedPanel = PanelThread
	}
}

// keepFocusOffHiddenMessages runs after every Update: a zoomed thread
// covers the messages pane, and plenty of handlers park focus there
// without knowing about the zoom (window split, hiding a focused
// sidebar). Normalizing here rather than at each of those keeps the
// invariant in one place: focus is never on a pane the frame won't
// draw.
func (a *App) keepFocusOffHiddenMessages() {
	if a.threadVisible && a.threadFullscreen && a.focusedPanel == PanelMessages {
		a.focusedPanel = PanelThread
	}
}
