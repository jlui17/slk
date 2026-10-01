package compose

import "slices"

// CursorOnFirstVisualRow reports whether the cursor is on the topmost
// row the text area draws: the first line, and the first row of it when
// the line is soft-wrapped. CursorAtFirstLine is true on every row of a
// wrapped first line.
func (m *Model) CursorOnFirstVisualRow() bool {
	return m.input.Line() == 0 && m.input.LineInfo().RowOffset == 0
}

// RemoveAttachmentAt removes the pending attachment at index i.
func (m *Model) RemoveAttachmentAt(i int) {
	m.pending = slices.Delete(m.pending, i, i+1)
	m.dirty()
}
