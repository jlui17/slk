package entrylist

import (
	"fmt"
	"testing"
)

// A scroll that leaves only an entry's trailing spacer rows at the top
// of the window moves the cursor past it, to the first entry whose own
// rows show: the spacer is not the message.
func TestCursorFollowsScrollPastTrail(t *testing.T) {
	m := New(WithCursorFollowsScroll())
	es := make([]Entry, 10)
	for i := range es {
		es[i] = msg(fmt.Sprintf("m%02d", i), i, 4, 2) // 2 message rows, 2 spacer rows
	}
	m.SetHeight(6, 0)
	m.SetEntries(es, 20, false)
	m.Layout(0, nil)
	m.ScrollDown(6, 0) // rows 6-7 are m01's spacer; m02 starts at row 8
	f := m.Layout(0, nil)
	if f.Cursor != 2 {
		t.Errorf("cursor %d, want 2: m01 shows only its spacer rows", f.Cursor)
	}
}
