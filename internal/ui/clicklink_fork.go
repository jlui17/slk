package ui

import tea "charm.land/bubbletea/v2"

// openPressedLink ends a plain click whose press landed on a link's
// text: it drops the press's empty selection, ends the drag, and opens
// the link in place of the click's usual outcome (the thread open).
// The link is read at press time because the press selects the message
// and the next frame can scroll it into view, moving the rows under the
// pointer before the release. nil, with the drag untouched, for a drag
// or a press off any link.
func (d *dragState) openPressedLink(a *App) tea.Cmd {
	if d.moved || d.link == "" {
		return nil
	}
	url := d.link
	switch d.panel {
	case PanelMessages:
		a.messagepane.ClearSelection()
	case PanelThread:
		a.threadPanel.ClearSelection()
	}
	d.Finish()
	return func() tea.Msg { return OpenLinkMsg{URL: url} }
}
