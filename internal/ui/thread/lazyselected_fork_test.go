package thread

// replySelectedRows returns the rows reply i of m draws as the cursor's,
// outside a text selection: built on demand (messages.LazySelected).
func replySelectedRows(m *Model, i int) []string {
	return m.cache[i].SelectedLines(&m.replyEntries[i], false, nil)
}
