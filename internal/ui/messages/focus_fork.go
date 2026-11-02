package messages

// refocusEntry renders entry i again when its selected rows carry the
// selection colours of the other focus. It runs as the entry is drawn
// selected (selectedLines), so it covers every way the cursor reaches an
// entry: SetFocused re-renders only the entry selected at the time, and
// an entry the cursor moves to later still carries the focus of the
// render that built it. Rendering again leaves the entry's height alone
// (focus changes colours only), so the list's offsets stay valid
// mid-layout.
func (m *Model) refocusEntry(i int) {
	if m.cache[i].focused == m.focused {
		return
	}
	entries := m.list.Entries()
	entries[i], m.cache[i] = m.renderMessageEntry(entries[i].Ref, m.cacheWidth, m.buildCacheStyles(m.cacheWidth), nil)
}
