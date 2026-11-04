package messages

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// tsAt is the Slack ts of 2026-09-29 at h:m:s local time.
func tsAt(h, m, s int) string {
	return fmt.Sprintf("%d.000100", time.Date(2026, 9, 29, h, m, s, 0, time.Local).Unix())
}

func TestContinues(t *testing.T) {
	alice := MessageItem{TS: tsAt(15, 0, 0), UserID: "U1", UserName: "alice"}
	at := func(m MessageItem, ts string) MessageItem { m.TS = ts; return m }
	nextDay := fmt.Sprintf("%d.000100", time.Date(2026, 9, 30, 0, 1, 0, 0, time.Local).Unix())
	cases := []struct {
		name       string
		prev, msg  MessageItem
		continuing bool
	}{
		{"same author a minute later", alice, at(alice, tsAt(15, 1, 0)), true},
		{"just inside the gap", alice, at(alice, tsAt(15, 4, 59)), true},
		{"at the gap", alice, at(alice, tsAt(15, 5, 0)), false},
		{"another author", alice, MessageItem{TS: tsAt(15, 1, 0), UserID: "U2", UserName: "bob"}, false},
		{"a bot under another name", MessageItem{TS: tsAt(15, 0, 0), UserID: "B1", UserName: "Claude [a]"},
			MessageItem{TS: tsAt(15, 1, 0), UserID: "B1", UserName: "Claude [b]"}, false},
		{"across midnight", at(alice, tsAt(23, 59, 0)), at(alice, nextDay), false},
		{"ephemeral before", MessageItem{TS: tsAt(15, 0, 0), UserID: "U1", UserName: "alice", Ephemeral: true}, at(alice, tsAt(15, 1, 0)), false},
		{"ephemeral", alice, MessageItem{TS: tsAt(15, 1, 0), UserID: "U1", UserName: "alice", Ephemeral: true}, false},
		{"a /me line", alice, MessageItem{TS: tsAt(15, 1, 0), UserID: "U1", UserName: "alice", Subtype: "me_message"}, false},
		{"a broadcast reply", alice, MessageItem{TS: tsAt(15, 1, 0), UserID: "U1", UserName: "alice", Subtype: "thread_broadcast"}, false},
		{"out of order", at(alice, tsAt(15, 1, 0)), alice, false},
	}
	for _, c := range cases {
		if got := Continues(c.prev, c.msg); got != c.continuing {
			t.Errorf("%s: Continues = %v, want %v", c.name, got, c.continuing)
		}
	}
}

func TestTextWidth(t *testing.T) {
	t.Cleanup(func() { SetMessageWidth(80) })
	for _, c := range []struct{ cap, pane, want int }{
		{80, 200, 80},  // the cap
		{80, 60, 50},   // the pane is narrower: what it leaves right of the gutter
		{0, 200, 190},  // no cap
		{-1, 200, 190}, // a negative cap is none too
		{80, 25, 20},   // never under 20
	} {
		SetMessageWidth(c.cap)
		if got := TextWidth(c.pane); got != c.want {
			t.Errorf("cap %d, pane %d: TextWidth = %d, want %d", c.cap, c.pane, got, c.want)
		}
	}
}

// Body text wraps at the cap in a wide pane, and at the pane with the
// cap off.
func TestMessageWidthWrapsTheBody(t *testing.T) {
	t.Cleanup(func() { SetMessageWidth(80) })
	msg := MessageItem{TS: tsAt(15, 0, 0), UserID: "U1", UserName: "alice", Timestamp: "3:00 PM", Text: strings.Repeat("word ", 60)}
	widest := func() int {
		m := New([]MessageItem{msg}, "general")
		w := 0
		for _, row := range strings.Split(ansi.Strip(m.View(40, 200)), "\n") {
			if strings.Contains(row, "word") {
				w = max(w, len(strings.TrimRight(row, " "))-len("▌")+1-TextCol)
			}
		}
		return w
	}
	if got := widest(); got > 80 || got < 70 {
		t.Errorf("capped body is %d cells wide, want at most 80", got)
	}
	SetMessageWidth(0)
	if got := widest(); got <= 80 {
		t.Errorf("uncapped body is %d cells wide, want it to use the pane", got)
	}
}

// groupedPane is a channel pane over: alice 3:00, alice 3:01 (grouped),
// bob 3:02, bob 3:10 (past the gap), alice 3:11.
func groupedPane(t *testing.T) (*Model, []MessageItem) {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	msgs := []MessageItem{
		{TS: tsAt(15, 0, 0), UserID: "U1", UserName: "alice", Timestamp: "3:00 PM", Text: "a-one"},
		{TS: tsAt(15, 1, 0), UserID: "U1", UserName: "alice", Timestamp: "3:01 PM", Text: "a-two"},
		{TS: tsAt(15, 2, 0), UserID: "U2", UserName: "bob", Timestamp: "3:02 PM", Text: "b-one"},
		{TS: tsAt(15, 10, 0), UserID: "U2", UserName: "bob", Timestamp: "3:10 PM", Text: "b-two"},
		{TS: tsAt(15, 11, 0), UserID: "U1", UserName: "alice", Timestamp: "3:11 PM", Text: "a-three"},
	}
	m := New(slices.Clone(msgs), "general")
	m.SetFocused(true)
	m.SelectByIndex(0) // keep the long timestamp off the rows asserted on
	return m, msgs
}

// paneRows is the pane's rows, stripped, with the selection column cut.
func paneRows(m *Model) []string {
	rows := strings.Split(ansi.Strip(m.View(60, 100)), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(strings.TrimPrefix(r, "▌"), " ")
	}
	return rows
}

func rowWith(t *testing.T, rows []string, s string) int {
	t.Helper()
	for i, r := range rows {
		if strings.Contains(r, s) {
			return i
		}
	}
	t.Fatalf("no row with %q:\n%s", s, strings.Join(rows, "\n"))
	return -1
}

func TestChannelPaneGroupsByAuthor(t *testing.T) {
	m, _ := groupedPane(t)
	rows := paneRows(m)
	two := rowWith(t, rows, "a-two")
	// A continuation: no name row, its time in the gutter on its body row.
	if want := " 3:01 a-two"; rows[two] != want {
		t.Errorf("continuation row = %q, want %q", rows[two], want)
	}
	if strings.Contains(rows[two-1], "alice") {
		t.Errorf("continuation has a name row: %q", rows[two-1])
	}
	// One blank row between messages from the same person.
	if one := rowWith(t, rows, "a-one"); two-one != 2 {
		t.Errorf("a-one at %d, a-two at %d: want one blank row between", one, two)
	}
	// Past the gap: a name row again, with the time after the name.
	if b2 := rowWith(t, rows, "b-two"); !strings.HasPrefix(strings.TrimSpace(rows[b2-1]), "bob  3:10 PM") {
		t.Errorf("past the gap the name row is %q", rows[b2-1])
	}
	// Two blank rows when the speaker changes.
	bob := rowWith(t, rows, "bob  3:02 PM")
	if bob-two != 3 || rows[two+1] != "" || rows[two+2] != "" {
		t.Errorf("a-two at %d, bob at %d: want two blank rows between", two, bob)
	}
}

// The new-messages line starts a group, and sits centred in its gap.
func TestChannelPaneNewMessagesLineBreaksAGroup(t *testing.T) {
	m, msgs := groupedPane(t)
	m.SetLastReadTS(msgs[0].TS)
	rows := paneRows(m)
	line := rowWith(t, rows, "─ new")
	if !strings.HasPrefix(rows[line], strings.Repeat(" ", TextCol)+"─") || !strings.HasSuffix(rows[line], "─ new") {
		t.Errorf("new-messages line = %q, want a rule from the text column ending in \" new\"", rows[line])
	}
	if rows[line-1] != "" || rows[line+1] != "" {
		t.Errorf("new-messages line is not centred in its gap:\n%s", strings.Join(rows[line-2:line+3], "\n"))
	}
	if !strings.Contains(rows[line+2], "alice  3:01 PM") {
		t.Errorf("the message under the line has no name row: %q", rows[line+2])
	}
}

// A continuation's "(edited)" follows its text, as Slack draws it.
func TestContinuationEditedMarkFollowsTheBody(t *testing.T) {
	m, msgs := groupedPane(t)
	edited := msgs[1]
	edited.IsEdited = true
	m.UpdateMessageInPlace(edited)
	rows := paneRows(m)
	if row := rows[rowWith(t, rows, "a-two")]; row != " 3:01 a-two (edited)" {
		t.Errorf("edited continuation row = %q", row)
	}
}

// Every way the grouping of a kept entry can change leaves the pane as
// a full rebuild would, rendering only the entries whose grouping or
// spacing changed.
func TestGroupingMatchesFullRebuild(t *testing.T) {
	cases := []struct {
		name     string
		event    func(m *Model, msgs []MessageItem)
		rendered func(msgs []MessageItem) []string
	}{
		{
			name:  "the read boundary splits a group",
			event: func(m *Model, msgs []MessageItem) { m.SetLastReadTS(msgs[0].TS) },
			// a-two gets a name row; a-one keeps its one blank row
			rendered: func(msgs []MessageItem) []string { return []string{msgs[1].TS} },
		},
		{
			name:  "removing a continuation",
			event: func(m *Model, msgs []MessageItem) { m.RemoveMessageByTS(msgs[1].TS) },
			// a-one now meets bob: two blank rows under it
			rendered: func(msgs []MessageItem) []string { return []string{msgs[0].TS} },
		},
		{
			name: "an append from the same author",
			event: func(m *Model, msgs []MessageItem) {
				m.AppendMessage(MessageItem{TS: tsAt(15, 12, 0), UserID: "U1", UserName: "alice", Timestamp: "3:12 PM", Text: "a-four"})
			},
			// a-three gains its spacer; a-four continues it
			rendered: func(msgs []MessageItem) []string { return []string{msgs[4].TS, tsAt(15, 12, 0)} },
		},
		{
			name:  "removing the first of a group",
			event: func(m *Model, msgs []MessageItem) { m.RemoveMessageByTS(msgs[0].TS) },
			// a-two leads its group now
			rendered: func(msgs []MessageItem) []string { return []string{msgs[1].TS} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, msgs := groupedPane(t)
			_ = m.View(prH, prW)
			before := slices.Clone(m.list.Entries())
			tc.event(m, msgs)
			_ = m.View(prH, prW)
			if got, want := rerendered(before, m.list.Entries()), tc.rendered(msgs); !slices.Equal(got, want) {
				t.Errorf("rendered %v again, want %v", got, want)
			}
			assertMatchesFullRebuild(t, m)
		})
	}
}

// A bot's bracketed task name keeps its text, in TextMuted after the
// name.
func TestAuthorTagIsMuted(t *testing.T) {
	styles.Apply("coffee", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	rc := New(nil, "general").renderContext()
	got := renderAuthor(MessageItem{UserID: "B1", UserName: "Claude [review queue colours]"}, rc)
	if ansi.Strip(got) != "Claude [review queue colours]" {
		t.Errorf("author = %q, want the name as Slack shows it", ansi.Strip(got))
	}
	name := styles.Username("B1", false).Render("Claude")
	if !strings.HasPrefix(got, name) {
		t.Errorf("name is not in the Name role: %q", got)
	}
	if want := fgANSIFor(styles.TextMuted); !strings.Contains(got[len(name):], strings.TrimSuffix(strings.TrimPrefix(want, "\x1b["), "m")) {
		t.Errorf("tag is not in TextMuted: %q", got)
	}
}

// The copy label of a block capped at the message width sits at the
// block's right end, and a click there copies the block.
func TestCodeBlockAt_CappedBlockInAWidePane(t *testing.T) {
	m := New([]MessageItem{{TS: "1.0", UserName: "alice", Text: "```\ncapped\n```"}}, "general")
	view := m.View(24, 200)
	var y, x = -1, -1
	for row, r := range strings.Split(ansi.Strip(view), "\n") {
		if before, _, ok := strings.Cut(r, copyLabelText); ok {
			y, x = row, ansi.StringWidth(before)
		}
	}
	if y < 0 {
		t.Fatalf("no copy label on screen:\n%s", ansi.Strip(view))
	}
	if want := TextCol + TextWidth(200) - len(copyLabelText); x != want {
		t.Errorf("copy label at column %d, want %d (the right end of an 80-cell block)", x, want)
	}
	if got, ok := m.CodeBlockAt(y, x+1); !ok || got != "capped" {
		t.Errorf("CodeBlockAt(label) = %q, %v; want the block's code", got, ok)
	}
}
