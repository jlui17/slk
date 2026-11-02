package messages

import "github.com/gammons/slk/internal/bubbles/entrylist"

// LazySelected is what an entry's selected rows are built from when its
// pane builds them on demand: only the cursor's entry is ever drawn
// selected, so a cache build renders no selected variant (a fifth of a
// full rebuild) and the cursor's entry builds its own when first drawn.
// The rows are the ones WrapRows builds, byte for byte: the same
// content, width and focus colours.
type LazySelected struct {
	ok      bool
	src     string // the content with the date-qualified header
	width   int
	focused bool     // the selection colours of the build
	rows    []string // built on first use
}

// DeferSelected returns s for a pane that builds selected rows on
// demand: WrapRows leaves WrappedRows.Selected nil and fills Lazy.
func (s WrapStyles) DeferSelected() WrapStyles {
	s.deferSelected = true
	return s
}

// selectedOrDeferred is WrapRows' selected variant, or with
// deferSelected, what builds it later.
func (s WrapStyles) selectedOrDeferred(renderedSel string, width int) ([]string, LazySelected) {
	if s.deferSelected {
		return nil, LazySelected{ok: true, src: renderedSel, width: width, focused: s.focused}
	}
	return s.SelectedVariant(renderedSel, width), LazySelected{}
}

// selectedRows returns e's selected rows: e.Selected, or the rows Lazy
// builds once, followed by the rows of e past the message (the channel
// pane's spacer), which every variant shares.
func (p *EntryPayload) selectedRows(e *entrylist.Entry) []string {
	l := &p.Lazy
	if e.Selected != nil || !l.ok {
		return e.Selected
	}
	if l.rows == nil {
		rows := NewWrapStyles(l.focused).SelectedVariant(l.src, l.width)
		if len(rows) <= len(e.Lines) { // always: variants share a height
			rows = append(rows, e.Lines[len(rows):]...)
		}
		l.rows = rows
	}
	return l.rows
}
