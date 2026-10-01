package ui

// toggleAttachmentFoldOfSelected is the z key: the selected message's
// long attachment cards expand, or collapse again. Each pane keeps its
// own state, so a message open in both folds in one at a time.
func (a *App) toggleAttachmentFoldOfSelected() {
	switch a.focusedPanel {
	case PanelMessages:
		a.messagepane.ToggleAttachmentFold()
	case PanelThread:
		a.threadPanel.ToggleAttachmentFold()
	}
}
