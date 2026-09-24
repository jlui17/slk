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

// SetMultiSelect turns row marking on for the picker Open just showed.
// Open and Close turn it back off.
func (m *Model) SetMultiSelect(on bool) { m.multiSelect = on }

// MultiSelect reports whether rows can be marked.
func (m *Model) MultiSelect() bool { return m.multiSelect }

// ToggleMark flips the mark on the highlighted row.
func (m *Model) ToggleMark() {
	if m.marked == nil {
		m.marked = make(map[int]bool)
	}
	m.marked[m.selected] = !m.marked[m.selected]
}

// ToggleMarkAll clears every mark when all rows are marked, and marks
// every row otherwise.
func (m *Model) ToggleMarkAll() {
	if len(m.Marked()) == len(m.items) {
		m.marked = nil
		return
	}
	m.marked = make(map[int]bool, len(m.items))
	for i := range m.items {
		m.marked[i] = true
	}
}

// Marked returns the marked rows in list order.
func (m *Model) Marked() []Item {
	var out []Item
	for i, it := range m.items {
		if m.marked[i] {
			out = append(out, it)
		}
	}
	return out
}

func (m *Model) resetMultiSelect() {
	m.multiSelect = false
	m.marked = nil
}
