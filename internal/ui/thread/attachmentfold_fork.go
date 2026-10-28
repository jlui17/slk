package thread

import "slices"

// redraw runs View at the size it last drew. Events drop the cache
// between draws, and the cache is built inside View; a key or a click
// that lands before the next draw must still find the fold rows.
func (m *Model) redraw() {
	if m.chromeWidth > 0 {
		m.View(m.list.Height()+m.chromeHeight, m.chromeWidth)
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
	if len(entry.AttachmentFoldRows) == 0 {
		return false
	}
	if m.expandedAttachments == nil {
		m.expandedAttachments = map[string]bool{}
	}
	m.expandedAttachments[msg.TS] = !m.expandedAttachments[msg.TS]
	// The message changes height: the next View brings it back into view.
	m.list.ResetSnap()
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
	line := y - m.chromeHeight + m.list.YOffset()
	_, start, payload, ok := m.forkEntryAt(line)
	if !ok || !slices.Contains(payload.AttachmentFoldRows, line-start) {
		return false
	}
	m.ClickAt(y)
	return m.ToggleAttachmentFold()
}
