package ui

import (
	tea "charm.land/bubbletea/v2"
)

// handleLinkPickerMarkKeys owns the keys a multi-select link picker
// adds: space marks the cursor row, a marks or clears all, and enter
// with anything marked opens the marked links as a batch of herdr tabs.
// Everything else (enter with nothing marked included) reports
// unhandled and takes handleLinkPickerMode's single-choice path.
func (a *App) handleLinkPickerMarkKeys(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !a.linkPicker.MultiSelect() {
		return nil, false
	}
	switch msg.String() {
	case "space":
		a.linkPicker.ToggleMark()
		return nil, true
	case "a":
		a.linkPicker.ToggleMarkAll()
		return nil, true
	case "enter":
		marked := a.linkPicker.Marked()
		if len(marked) == 0 {
			return nil, false
		}
		urls := make([]string, len(marked))
		for i, it := range marked {
			urls[i] = it.URL
		}
		a.linkPicker.Close()
		a.SetMode(ModeNormal)
		a.pickerInTab = false
		return func() tea.Msg { return OpenLinksInHerdrTabsMsg{URLs: urls} }, true
	}
	return nil, false
}
