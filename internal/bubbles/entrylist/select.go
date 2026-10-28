package entrylist

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/selection"
)

// AbsLine maps a content row (0 = first row under the owner's chrome) to
// an absolute line, clamped to the content.
func (m *Model) AbsLine(contentY int) int {
	abs := max(contentY, 0) + m.yOffset
	abs = max(abs, 0)
	if m.total > 0 && abs >= m.total {
		abs = m.total - 1
	}
	return abs
}

// EntryAt returns the entry covering absolute line abs and the row within
// it, among the entries lookups may see (KeepLookups).
func (m *Model) EntryAt(abs int) (idx, within int, ok bool) {
	for i := range m.lookupLen() {
		start := m.offsets[i]
		if abs >= start && abs < start+len(m.entries[i].Lines) {
			return i, abs - start, true
		}
	}
	return 0, 0, false
}

// HasSelection reports whether a text selection is active or pinned.
func (m *Model) HasSelection() bool { return m.hasSel }

// Selection returns the current selection range.
func (m *Model) Selection() selection.Range { return m.sel }

// BeginSelection anchors a new selection at absolute line abs, display
// column col. A press that anchors nothing is a no-op.
func (m *Model) BeginSelection(abs, col int) {
	a, ok := m.anchorAt(abs, col)
	if !ok {
		return
	}
	if a, ok = m.snapAnchor(a, abs); !ok {
		return
	}
	m.sel = selection.Range{Start: a, End: a, Active: true}
	if m.dirtyOnDrag || !m.hasSel {
		// The channel pane bumps once per drag: the selected row swaps
		// to its short-timestamp variant. The thread pane bumps on every
		// change (WithDirtyOnDrag).
		m.version++
	}
	m.hasSel = true
}

// ExtendSelection moves the selection's end.
func (m *Model) ExtendSelection(abs, col int) {
	if !m.hasSel {
		return
	}
	a, ok := m.anchorAt(abs, col)
	if !ok {
		return
	}
	if a, ok = m.snapAnchor(a, abs); !ok {
		return
	}
	if a == m.sel.End {
		return
	}
	m.sel.End = a
	if m.dirtyOnDrag {
		m.version++
	}
}

// EndSelection finishes a drag and returns the selected text; ok is false
// when nothing was selected. An empty selection is cleared.
func (m *Model) EndSelection() (string, bool) {
	if !m.hasSel {
		return "", false
	}
	m.sel.Active = false
	if m.sel.IsEmpty() {
		m.hasSel = false
		m.sel = selection.Range{}
		m.version++
		return "", false
	}
	text := m.SelectionText()
	if m.dirtyOnDrag {
		m.version++
	}
	if text == "" {
		return "", false
	}
	return text, true
}

// ClearSelection removes the selection, if any.
func (m *Model) ClearSelection() {
	if !m.hasSel {
		return
	}
	m.hasSel = false
	m.sel = selection.Range{}
	m.version++
}

// SelectionText returns the selected text: each row trimmed of trailing
// spaces, rows joined by newlines, no trailing newline.
func (m *Model) SelectionText() string {
	if !m.hasSel || m.sel.IsEmpty() {
		return ""
	}
	loLine, loCol, hiLine, hiCol, ok := m.selectedSpan()
	if !ok {
		return ""
	}
	var b strings.Builder
	for i := range m.lookupLen() {
		e := &m.entries[i]
		start := m.offsets[i]
		if start+len(e.Lines) <= loLine {
			continue
		}
		if start > hiLine {
			break
		}
		for j, plain := range e.Plain {
			abs := start + j
			if abs < loLine {
				continue
			}
			if abs > hiLine {
				break
			}
			from, to := 0, DisplayWidth(plain)
			if abs == loLine {
				from = loCol
			}
			if abs == hiLine {
				to = hiCol
			}
			b.WriteString(strings.TrimRight(SliceColumns(plain, from, to), " "))
			if abs != hiLine {
				b.WriteByte('\n')
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ApplySelectionToBordered paints the selection onto an owner's cached,
// bordered frame: startRow is the frame row of the first content row
// (border plus chrome rows above it), leftCols the border columns.
func (m *Model) ApplySelectionToBordered(bordered string, startRow, leftCols int) string {
	if !m.hasSel || m.height <= 0 {
		return bordered
	}
	lines := strings.Split(bordered, "\n")
	m.overlaySelection(lines, startRow, leftCols)
	return strings.Join(lines, "\n")
}

// selectedSpan resolves the normalized selection to absolute lines and
// plain columns; ok is false when either end no longer resolves or the
// span is empty.
func (m *Model) selectedSpan() (loLine, loCol, hiLine, hiCol int, ok bool) {
	lo, hi := m.sel.Normalize()
	loLine, loCol, ok1 := m.Resolve(lo)
	hiLine, hiCol, ok2 := m.Resolve(hi)
	if !ok1 || !ok2 || loLine > hiLine || (loLine == hiLine && loCol >= hiCol) {
		return 0, 0, 0, 0, false
	}
	return loLine, loCol, hiLine, hiCol, true
}

// overlaySelection highlights the selected columns of the window rows in
// lines, which start at index startRow; columns shift by leftCols. Rows
// Compose replaced with indicators are skipped.
func (m *Model) overlaySelection(lines []string, startRow, leftCols int) {
	loLine, loCol, hiLine, hiCol, ok := m.selectedSpan()
	if !ok {
		return
	}
	for vRow := 0; vRow < m.height; vRow++ {
		row := startRow + vRow
		if row < 0 || row >= len(lines) {
			continue
		}
		if (vRow == 0 && m.overTop) || (vRow == m.height-1 && m.overBottom) {
			continue
		}
		abs := m.yOffset + vRow
		if abs < loLine || abs > hiLine {
			continue
		}
		i, j, found := m.EntryAt(abs)
		if !found {
			continue
		}
		e := &m.entries[i]
		if j >= len(e.Plain) {
			continue
		}
		plain := e.Plain[j]
		from, to := 0, DisplayWidth(plain)
		if abs == loLine {
			from = loCol
		}
		if abs == hiLine {
			to = hiCol
		}
		from, to = max(from, 0), min(to, DisplayWidth(plain))
		if from >= to {
			continue
		}
		styled := lines[row]
		dispFrom := from + e.ColOffset + leftCols
		dispTo := to + e.ColOffset + leftCols
		w := ansi.StringWidth(styled)
		if dispFrom >= w {
			continue
		}
		dispTo = min(dispTo, w)
		lines[row] = ansi.Cut(styled, 0, dispFrom) +
			m.styles.Selection.Render(SliceColumns(plain, from, to)) +
			ansi.Cut(styled, dispTo, w)
	}
}

func (m *Model) anchorAt(abs, col int) (selection.Anchor, bool) {
	i, within, ok := m.EntryAt(abs)
	if !ok {
		return selection.Anchor{}, false
	}
	e := &m.entries[i]
	if within >= len(e.Plain) {
		return selection.Anchor{}, false
	}
	plainCol := min(max(col-e.ColOffset, 0), DisplayWidth(e.Plain[within]))
	return selection.Anchor{MessageID: e.ID, Line: within, Col: plainCol}, true
}

// snapAnchor moves an anchor on a decoration (empty MessageID) to the
// next selectable entry's first row, or failing that the previous one's
// last row. Without WithSnapDecorationAnchors anchors pass through.
func (m *Model) snapAnchor(a selection.Anchor, abs int) (selection.Anchor, bool) {
	if !m.snapDecor || a.MessageID != "" {
		return a, true
	}
	start, _, ok := m.EntryAt(abs)
	if !ok {
		return selection.Anchor{}, false
	}
	for i := start + 1; i < m.lookupLen(); i++ {
		if e := &m.entries[i]; e.Selectable {
			return selection.Anchor{MessageID: e.ID}, true
		}
	}
	for i := start - 1; i >= 0; i-- {
		if e := &m.entries[i]; e.Selectable {
			last := len(e.Lines) - 1
			col := 0
			if last < len(e.Plain) {
				col = DisplayWidth(e.Plain[last])
			}
			return selection.Anchor{MessageID: e.ID, Line: last, Col: col}, true
		}
	}
	return selection.Anchor{}, false
}

// Resolve returns the absolute line and plain column an anchor points
// at; ok is false when its entry is gone (or hidden by KeepLookups).
func (m *Model) Resolve(a selection.Anchor) (abs, col int, ok bool) {
	if a.MessageID == "" {
		return 0, 0, false
	}
	i, found := m.ids[a.MessageID]
	if !found || i >= m.lookupLen() {
		return 0, 0, false
	}
	if a.Line < 0 || a.Line >= len(m.entries[i].Plain) {
		return 0, 0, false
	}
	return m.offsets[i] + a.Line, a.Col, true
}
