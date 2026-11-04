package thread

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

func TestRenderThreadHeader(t *testing.T) {
	cases := []struct {
		name, channel, chType string
		width                 int
		want                  string
		cols                  int
	}{
		{"room for everything", "general", "channel", 40, hinted("# general › Thread", 40), 9},
		{"hint fits with a two-space gap", "general", "channel", 29, "# general › Thread  esc close", 9},
		{"one short of the hint drops it", "general", "channel", 28, pad("# general › Thread", 28), 9},
		{"channel truncated", "general", "channel", 13, "# g… › Thread", 4},
		{"no room for any channel", "general", "channel", 10, pad("Thread", 10), 0},
		{"no channel", "", "", 40, hinted("Thread", 40), 0},
		{"private glyph", "design", "private", 40, hinted("◆ design › Thread", 40), 8},
	}
	for _, c := range cases {
		got, cols := renderThreadHeader(c.width, c.channel, c.chType)
		if ansi.Strip(got) != c.want || cols != c.cols {
			t.Errorf("%s: got %q cols %d, want %q cols %d", c.name, ansi.Strip(got), cols, c.want, c.cols)
		}
		if w := ansi.StringWidth(got); w != c.width {
			t.Errorf("%s: width %d, want %d", c.name, w, c.width)
		}
	}
}

func threadTS(minute int) string {
	return fmt.Sprintf("%d.000100", time.Date(2026, 9, 29, 15, minute, 0, 0, time.Local).Unix())
}

// groupedThread: alice's parent at 3:00, then bob 3:01, bob 3:02
// (grouped), carol 3:03, bob 3:04.
func groupedThread(t *testing.T) (*Model, messages.MessageItem, []messages.MessageItem) {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	messages.SetNowFunc(threadLongTSClock)
	t.Cleanup(func() { messages.SetNowFunc(nil); styles.Apply("dark", config.Theme{}) })
	parent := messages.MessageItem{TS: threadTS(0), UserID: "U1", UserName: "alice", Text: "the parent", Timestamp: "3:00 PM"}
	replies := []messages.MessageItem{
		{TS: threadTS(1), UserID: "U2", UserName: "bob", Text: "b-one", Timestamp: "3:01 PM"},
		{TS: threadTS(2), UserID: "U2", UserName: "bob", Text: "b-two", Timestamp: "3:02 PM"},
		{TS: threadTS(3), UserID: "U3", UserName: "carol", Text: "c-one", Timestamp: "3:03 PM"},
		{TS: threadTS(4), UserID: "U2", UserName: "bob", Text: "b-three", Timestamp: "3:04 PM"},
	}
	m := New()
	m.SetThread(parent, slices.Clone(replies), "C1", parent.TS)
	m.SetFocused(true)
	m.GoToTop() // the parent: keep the long timestamp off the replies
	return m, parent, replies
}

func threadRows(m *Model) []string {
	rows := strings.Split(ansi.Strip(m.View(40, 100)), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(strings.TrimPrefix(r, "▌"), " ")
	}
	return rows
}

func threadRowWith(t *testing.T, rows []string, s string) int {
	t.Helper()
	for i, r := range rows {
		if strings.Contains(r, s) {
			return i
		}
	}
	t.Fatalf("no row with %q:\n%s", s, strings.Join(rows, "\n"))
	return -1
}

func TestThreadLayout(t *testing.T) {
	m, _, _ := groupedThread(t)
	m.MoveDown() // off the parent, so it shows its date, not the long form
	rows := threadRows(m)
	if rows[0] != hinted("Thread", 100) {
		t.Errorf("header = %q", rows[0])
	}
	// The parent: its time in the margin, its date after the name.
	parent := threadRowWith(t, rows, "alice")
	if rows[parent] != " 3:00 alice  Today" {
		t.Errorf("parent header = %q", rows[parent])
	}
	// A blank row, then the replies line in the text column.
	replies := threadRowWith(t, rows, "4 replies ")
	if rows[replies-1] != "" || !strings.HasPrefix(rows[replies], strings.Repeat(" ", messages.TextCol)+"4 replies ─") {
		t.Errorf("replies line = %q above %q", rows[replies], rows[replies-1])
	}
	// A continuation: its time in the margin on its body row.
	two := threadRowWith(t, rows, "b-two")
	if rows[two] != " 3:02 b-two" {
		t.Errorf("continuation = %q", rows[two])
	}
	// One blank row for the same person, two when the speaker changes.
	if one := threadRowWith(t, rows, "b-one"); two-one != 2 {
		t.Errorf("b-one at %d, b-two at %d: want one blank row between", one, two)
	}
	if carol := threadRowWith(t, rows, "3:03 carol"); carol-two != 3 {
		t.Errorf("b-two at %d, carol at %d: want two blank rows between", two, carol)
	}
	// No rule between replies.
	for _, r := range rows[replies+1:] {
		if s := strings.TrimSpace(r); s != "" && strings.Trim(s, "─") == "" {
			t.Errorf("a rule between replies: %q", r)
		}
	}
}

// Moving the unread boundary into a group, or a reply going between
// two of one person's, redraws only the replies whose grouping changed,
// as a full rebuild would draw them.
func TestThreadGroupingMatchesFullRebuild(t *testing.T) {
	cases := []struct {
		name     string
		event    func(m *Model, replies []messages.MessageItem)
		rendered func(replies []messages.MessageItem) []string
	}{
		{
			name:     "the unread boundary splits a group",
			event:    func(m *Model, r []messages.MessageItem) { m.SetLastReadTS(r[0].TS) },
			rendered: func(r []messages.MessageItem) []string { return []string{r[1].TS} },
		},
		{
			name:     "a removal joins two of bob's",
			event:    func(m *Model, r []messages.MessageItem) { m.RemoveMessageByTS(r[2].TS) },
			rendered: func(r []messages.MessageItem) []string { return []string{r[3].TS} },
		},
		{
			name: "a reply from the last speaker",
			event: func(m *Model, r []messages.MessageItem) {
				m.AddReply(messages.MessageItem{TS: threadTS(5), UserID: "U2", UserName: "bob", Text: "b-four", Timestamp: "3:05 PM"})
			},
			// The cursor follows the new reply off the parent.
			rendered: func(r []messages.MessageItem) []string { return []string{threadTS(0), threadTS(5)} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, replies := groupedThread(t)
			_ = m.View(orH, orW)
			before := slices.Clone(m.list.Entries())
			tc.event(m, replies)
			_ = m.View(orH, orW)
			if got, want := orRerendered(before, m.list.Entries()), tc.rendered(replies); !slices.Equal(got, want) {
				t.Errorf("rendered %v again, want %v", got, want)
			}
			assertMatchesFullRebuild(t, m)
		})
	}
}
