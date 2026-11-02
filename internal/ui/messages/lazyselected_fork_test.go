package messages

import (
	"slices"
	"testing"

	"github.com/gammons/slk/internal/bubbles/entrylist"
)

// entrySelectedRows returns the rows entry i of m draws as the cursor's,
// outside a text selection.
func entrySelectedRows(m *Model, i int) []string {
	return m.cache[i].SelectedLines(&m.list.Entries()[i], false, nil)
}

// The selected rows a pane builds on demand are the ones WrapRows builds
// up front, byte for byte, followed by the rows the entry adds past the
// message (the channel pane's spacer).
func TestLazySelected_MatchesWrapRows(t *testing.T) {
	msgs := benchRichMessages(t, 20)
	m := New(msgs, "general")
	spacer := "\x1b[48;2;1;2;3m \x1b[m"
	for _, focused := range []bool{true, false} {
		s := NewWrapStyles(focused)
		for i, msg := range msgs {
			r := RenderMessage(msg, benchEventW, m.renderContext(), RenderOptions{ReplyAffordance: true, EditedMark: true})
			want := WrapRows(msg, r, benchEventW, s).Selected
			lazy := WrapRows(msg, r, benchEventW, s.DeferSelected())
			if lazy.Selected != nil {
				t.Fatalf("msg %d: a deferring WrapRows built the selected rows", i)
			}
			e := entrylist.Entry{Lines: append(slices.Clone(lazy.Lines), spacer)}
			p := EntryPayload{Lazy: lazy.Lazy}
			if got := p.SelectedLines(&e, false, nil); !slices.Equal(got, append(slices.Clone(want), spacer)) {
				t.Errorf("msg %d focused=%v: built on demand\n got: %q\nwant: %q", i, focused, got, want)
			}
		}
	}
}
