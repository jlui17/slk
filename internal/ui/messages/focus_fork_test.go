package messages

import (
	"slices"
	"testing"
)

// After the pane's focus changes at the same width, every message the
// cursor reaches draws the selection colours of the new focus, as a
// full rebuild would, and only the messages drawn selected render again.
// The view is compared, not the entries: an entry's selected rows are
// brought to the new focus when it is drawn selected, so an entry the
// cursor never reached keeps rows that are never drawn.
func TestSetFocused_CursorMovedLaterDrawsTheNewFocus(t *testing.T) {
	for _, focused := range []bool{true, false} {
		msgs := prMessages(7)
		m := New(slices.Clone(msgs), "general")
		m.SetFocused(!focused)
		_ = m.View(prH, prW)
		before := slices.Clone(m.list.Entries())

		m.SetFocused(focused)
		_ = m.View(prH, prW)
		m.MoveUp()
		got := m.View(prH, prW)

		if r, want := rerendered(before, m.list.Entries()), []string{msgs[5].TS, msgs[6].TS}; !slices.Equal(r, want) {
			t.Errorf("focused=%v: rendered %v again, want %v", focused, r, want)
		}
		assertViewMatchesFullRebuild(t, m, got)
	}
}

// The same across a width round trip (a thread opened and closed) with
// the focus changed while the pane was narrow: entries kept from before
// the round trip carry the old focus too.
func TestSetFocused_ChangedWhileNarrowDrawsTheNewFocus(t *testing.T) {
	for _, focused := range []bool{true, false} {
		m := New(prMessages(7), "general")
		m.SetFocused(!focused)
		_ = m.View(prH, prW)
		_ = m.View(prH, prW/2)
		m.SetFocused(focused)
		_ = m.View(prH, prW/2)
		_ = m.View(prH, prW)
		m.MoveUp()
		assertViewMatchesFullRebuild(t, m, m.View(prH, prW))
	}
}

func assertViewMatchesFullRebuild(t *testing.T, m *Model, got string) {
	t.Helper()
	m.InvalidateCache()
	if want := m.View(prH, prW); got != want {
		t.Errorf("focused=%v: view differs from a full rebuild\n got: %q\nwant: %q", m.focused, got, want)
	}
}
