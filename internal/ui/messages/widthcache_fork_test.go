package messages

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/peerstatus"
)

const wideW, narrowW = 100, 70

// A thread opening and closing switches the channel pane between two
// widths. The cache of the other width is restored, and every change to
// a message made while it was put away reaches it: the pane at the
// restored width matches a full rebuild, rendering only the entries that
// changed. A change to the message list renders every entry again.
func TestModel_OtherWidthCacheMatchesFullRebuild(t *testing.T) {
	base := prMessages(7) // U1 wrote the even ones, U2 the odd ones
	base[3].Reactions = []ReactionItem{{Emoji: "eyes", Count: 1}}
	base[6].Reactions = []ReactionItem{{Emoji: "eyes", Count: 1}}
	ts := tsOf(base)
	newMsg := MessageItem{TS: "1700400000.000100", UserID: "U3", UserName: "carol", Text: "new", Timestamp: "11:00 AM"}

	cases := []struct {
		name     string
		narrow   func(m *Model) // while the pane is narrow
		rendered []string       // entries of the wide width rendered again
	}{
		{name: "nothing changes"},
		{
			name:     "focus moves to the thread and back",
			narrow:   func(m *Model) { m.SetFocused(false); _ = m.View(prH, narrowW); m.SetFocused(true) },
			rendered: []string{ts[6]},
		},
		{name: "a reaction", narrow: func(m *Model) { m.UpdateReaction(ts[3], "tada", "U9", false) }, rendered: []string{ts[3]}},
		{
			name:     "a reaction drawn at the narrow width",
			narrow:   func(m *Model) { m.UpdateReaction(ts[3], "tada", "U9", false); _ = m.View(prH, narrowW) },
			rendered: []string{ts[3]},
		},
		{name: "a new message", narrow: func(m *Model) { m.AppendMessage(newMsg) }, rendered: append(ts, newMsg.TS)},
		{
			name:     "a new message drawn at the narrow width",
			narrow:   func(m *Model) { m.AppendMessage(newMsg); _ = m.View(prH, narrowW) },
			rendered: append(ts, newMsg.TS),
		},
		{
			name:     "a deleted message",
			narrow:   func(m *Model) { m.RemoveMessageByTS(ts[2]) },
			rendered: slices.Delete(slices.Clone(ts), 2, 3),
		},
		{name: "an author's name", narrow: func(m *Model) { m.PatchUserName("U2", "bobby") }, rendered: []string{ts[1], ts[3], ts[5]}},
		{
			name:     "an author's status",
			narrow:   func(m *Model) { m.PatchUserStatus("U1", peerstatus.Status{Emoji: ":calendar:"}) },
			rendered: []string{ts[0], ts[2], ts[4], ts[6]},
		},
		{name: "a mark from another client", narrow: func(m *Model) { m.SetLastReadTS(ts[4]) }, rendered: ts},
		{name: "the fetch lands unchanged", narrow: func(m *Model) { m.SetMessages(slices.Clone(base)) }, rendered: ts},
		{name: "reaction nav", narrow: func(m *Model) { m.EnterReactionNav() }, rendered: []string{ts[6]}},
		{name: "the theme changes", narrow: func(m *Model) { m.InvalidateCache() }, rendered: ts},
		{
			name: "an ephemeral replaced",
			narrow: func(m *Model) {
				m.ReplaceEphemeral(MessageItem{TS: ts[5], UserID: "U2", UserName: "bob", Text: "replaced", Timestamp: "10:30 AM"})
			},
			rendered: ts,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(slices.Clone(base), "general")
			m.SetFocused(true)
			_ = m.View(prH, narrowW)
			_ = m.View(prH, wideW)
			before := slices.Clone(m.list.Entries())

			_ = m.View(prH, narrowW)
			if tc.narrow != nil {
				tc.narrow(m)
			}
			_ = m.View(prH, wideW)

			if got := rerendered(before, m.list.Entries()); !slices.Equal(got, tc.rendered) {
				t.Errorf("rendered %v again, want %v", got, tc.rendered)
			}
			assertMatchesFullRebuildAt(t, m, wideW)
		})
	}
}

// assertMatchesFullRebuildAt is assertMatchesFullRebuild at width w.
func assertMatchesFullRebuildAt(t *testing.T, m *Model, w int) {
	t.Helper()
	got := m.View(prH, w)
	gotEntries := slices.Clone(m.list.Entries())
	gotSelected := allSelectedRows(m)
	m.InvalidateCache()
	if want := m.View(prH, w); got != want {
		t.Errorf("view differs from a full rebuild\n got: %q\nwant: %q", got, want)
	}
	if !reflect.DeepEqual(gotEntries, m.list.Entries()) {
		t.Error("entries differ from a full rebuild")
	}
	if !reflect.DeepEqual(gotSelected, allSelectedRows(m)) {
		t.Error("selected rows differ from a full rebuild")
	}
}

// tallMessages returns n messages, ts from base a minute apart, message
// i rows(i) lines tall.
func tallMessages(base, n int, rows func(i int) int) []MessageItem {
	msgs := make([]MessageItem, n)
	for i := range msgs {
		msgs[i] = MessageItem{
			TS:        fmt.Sprintf("%d.000100", base+i*60),
			UserName:  "alice",
			UserID:    "U1",
			Text:      fmt.Sprintf("message %d", i) + strings.Repeat("\nline", rows(i)),
			Timestamp: "10:30 AM",
		}
	}
	return msgs
}

// paneState is what the channel pane shows at one width.
type paneState struct {
	frame    string
	yOffset  int
	selected string // the cursor's message ts
	entries  []entrylist.Entry
	rows     [][]string // allSelectedRows
}

func capturePane(m *Model, h, w int) paneState {
	frame := m.View(h, w)
	sel, _ := m.SelectedMessage()
	return paneState{frame, m.YOffset(), sel.TS, slices.Clone(m.list.Entries()), allSelectedRows(m)}
}

// A thread opened and closed after the message list changed (another
// channel, an older page, a new message, a channel left and come back
// to) leaves the channel pane as a pane that never opened a thread: at
// each width the same frame, scroll position, entries and cursor rows,
// and the cursor on the message it was on.
func TestModel_OtherWidthAfterMessagesChangeMatchesFreshPane(t *testing.T) {
	const h = 40
	general := tallMessages(1700000000, 60, func(i int) int { return 2 + (i%5)*6 })
	eng := tallMessages(1800000000, 50, func(i int) int {
		if i%7 == 3 {
			return 40
		}
		return 2
	})
	edited := slices.Clone(eng)
	edited[30].Text = "edited while the pane showed another channel"
	edited[30].Reactions = []ReactionItem{{Emoji: "eyes", Count: 1}}

	// thread opens (narrow, the thread focused), runs between, and closes.
	thread := func(m *Model, between func()) {
		m.SetFocused(false)
		_ = m.View(h, narrowW)
		if between != nil {
			between()
			_ = m.View(h, narrowW)
		}
		m.SetFocused(true)
		_ = m.View(h, wideW)
	}
	cases := []struct {
		name string
		// load fills m, calling thread(between) where a thread opens and
		// closes; the fresh pane runs between alone.
		load func(m *Model, thread func(between func()))
	}{
		{name: "another channel", load: func(m *Model, thread func(func())) {
			m.SetMessages(slices.Clone(general))
			_ = m.View(h, wideW)
			thread(nil)
			m.SetMessages(slices.Clone(eng))
		}},
		{name: "an older page", load: func(m *Model, thread func(func())) {
			m.SetMessages(slices.Clone(eng[25:]))
			_ = m.View(h, wideW)
			thread(nil)
			m.PrependMessages(slices.Clone(eng[:25]))
		}},
		{name: "a new message", load: func(m *Model, thread func(func())) {
			m.SetMessages(slices.Clone(eng[:49]))
			_ = m.View(h, wideW)
			thread(nil)
			m.AppendMessage(eng[49])
		}},
		{name: "a new message while a thread is open", load: func(m *Model, thread func(func())) {
			m.SetMessages(slices.Clone(eng[:49]))
			_ = m.View(h, wideW)
			thread(nil)
			thread(func() { m.AppendMessage(eng[49]) })
		}},
		{name: "a channel left and come back to, a message changed", load: func(m *Model, thread func(func())) {
			m.SetMessages(slices.Clone(eng))
			_ = m.View(h, wideW)
			thread(nil)
			m.SetMessages(slices.Clone(general))
			_ = m.View(h, wideW)
			m.SetMessages(slices.Clone(edited))
		}},
		{name: "a thread between every change", load: func(m *Model, thread func(func())) {
			m.SetMessages(slices.Clone(general))
			_ = m.View(h, wideW)
			thread(nil)
			m.SetMessages(slices.Clone(eng[25:49]))
			_ = m.View(h, wideW)
			thread(nil)
			m.PrependMessages(slices.Clone(eng[:25]))
			_ = m.View(h, wideW)
			thread(func() { m.AppendMessage(eng[49]) })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// run loads a pane, moves the cursor up, and opens and closes
			// a thread, returning the pane before, while and after.
			run := func(withThreads bool) (before, open, closed paneState) {
				m := New(nil, "eng")
				m.SetFocused(true)
				tc.load(m, func(between func()) {
					if withThreads {
						thread(m, between)
					} else if between != nil {
						between()
					}
				})
				_ = m.View(h, wideW)
				for range 30 {
					m.MoveUp()
					_ = m.View(h, wideW)
				}
				before = capturePane(m, h, wideW)
				m.SetFocused(false)
				open = capturePane(m, h, narrowW)
				m.SetFocused(true)
				closed = capturePane(m, h, wideW)
				return before, open, closed
			}
			fBefore, fOpen, fClosed := run(false)
			before, open, closed := run(true)
			for _, c := range []struct {
				step      string
				got, want paneState
			}{
				{"before the thread opens", before, fBefore},
				{"with the thread open", open, fOpen},
				{"after the thread closes", closed, fClosed},
			} {
				if c.got.selected != before.selected {
					t.Errorf("%s: cursor on %s, was on %s", c.step, c.got.selected, before.selected)
				}
				if c.got.yOffset != c.want.yOffset {
					t.Errorf("%s: scrolled to line %d, a fresh pane to %d", c.step, c.got.yOffset, c.want.yOffset)
				}
				if c.got.frame != c.want.frame {
					t.Errorf("%s: frame differs from a fresh pane's", c.step)
				}
				if !reflect.DeepEqual(c.got.entries, c.want.entries) {
					t.Errorf("%s: entries differ from a fresh pane's", c.step)
				}
				if !reflect.DeepEqual(c.got.rows, c.want.rows) {
					t.Errorf("%s: cursor rows differ from a fresh pane's", c.step)
				}
			}
		})
	}
}
