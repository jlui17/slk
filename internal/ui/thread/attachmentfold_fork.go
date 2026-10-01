package thread

import "slices"

func (m *Model) takeAttachmentFoldRows() []int {
	rows := m.attachmentFoldRows
	m.attachmentFoldRows = nil
	return rows
}

// redraw runs View at the size it last drew. Events drop the cache
// between draws, and the cache is built inside View; a key or a click
// that lands before the next draw must still find the fold rows.
func (m *Model) redraw() {
	if m.chromeWidth > 0 {
		m.View(m.lastViewHeight+m.chromeHeight, m.chromeWidth)
	}
}

// ToggleAttachmentFold expands or collapses every attachment card of
// the selected reply, or of the parent when it is selected. It reports
// false, and does nothing, when that message has no card long enough
// to fold.
func (m *Model) ToggleAttachmentFold() bool {
	m.redraw()
	msg := m.SelectedReply()
	if msg == nil {
		return false
	}
	entry := &m.parentEntry
	if m.selected != parentSelected {
		if m.selected >= len(m.cache) {
			return false
		}
		entry = &m.cache[m.selected]
	}
	if len(entry.attachmentFoldRows) == 0 {
		return false
	}
	if m.expandedAttachments == nil {
		m.expandedAttachments = map[string]bool{}
	}
	m.expandedAttachments[msg.TS] = !m.expandedAttachments[msg.TS]
	// The message changes height: the next View brings it back into view.
	m.hasSnapped = false
	m.InvalidateCache()
	return true
}

// ToggleAttachmentFoldAt handles a click at pane-local row y, the frame
// ClickAt takes: on a card's fold row it selects that message and
// toggles its cards.
func (m *Model) ToggleAttachmentFoldAt(y int) bool {
	if y < m.chromeHeight {
		return false
	}
	m.redraw()
	line := m.absoluteLineAt(y)
	entry, start, _, ok := m.selectionEntryAt(line)
	if !ok || !slices.Contains(entry.attachmentFoldRows, line-start) {
		return false
	}
	m.ClickAt(y)
	return m.ToggleAttachmentFold()
}
