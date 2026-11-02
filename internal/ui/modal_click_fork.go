package ui

import tea "charm.land/bubbletea/v2"

// dismissOr is a click outside the modal's box: the modal's own
// dismiss, when it has one, else esc.
func (t modalClickTarget) dismissOr(esc func() tea.Cmd) tea.Cmd {
	if t.dismiss != nil {
		return t.dismiss()
	}
	return esc()
}
