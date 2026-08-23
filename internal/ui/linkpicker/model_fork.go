package linkpicker

// SetDisplay replaces row index's Display text in place (async
// preview fill) and drops its Detail: the preview distinguishes the
// row, so the muted URL suffix has done its job. Out-of-range indexes
// are ignored.
func (m *Model) SetDisplay(index int, display string) {
	if index < 0 || index >= len(m.items) {
		return
	}
	m.items[index].Display = display
	m.items[index].Detail = ""
}
