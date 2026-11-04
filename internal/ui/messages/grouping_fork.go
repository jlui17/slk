package messages

import (
	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/styles"
)

// The channel pane's grouping and spacing (layout_fork.go has the
// rules). An entry depends on its neighbours now: whether it continues
// the message above, and how many blank rows follow it. Both are
// recorded on the entry (EntryPayload.Continuation, Entry.Trail), and a
// rebuild that keeps entries keeps only those whose neighbours still
// give the same answers (groupingChanged).

// continues reports whether message i is drawn as a continuation of
// i-1: grouped by Continues, with no day divider (Continues compares
// days) or new-messages line between them.
func (m *Model) continues(i int) bool {
	return i > 0 && !m.newLineBefore(i) && Continues(m.messages[i-1], m.messages[i])
}

// newLineBefore reports whether the new-messages line sits right above
// message i: the first message past the read boundary.
func (m *Model) newLineBefore(i int) bool {
	l := m.lastReadTS
	return l != "" && m.messages[i].TS > l && (i == 0 || m.messages[i-1].TS <= l)
}

// trailRows is how many blank rows follow message i: none after the
// last, one before a divider (the divider brings the one below it),
// else SpacerRows.
func (m *Model) trailRows(i int) int {
	if i >= len(m.messages)-1 {
		return 0
	}
	next := m.messages[i+1]
	if m.newLineBefore(i+1) || DateFromTS(next.TS) != DateFromTS(m.messages[i].TS) {
		return 1
	}
	return SpacerRows(m.messages[i], next)
}

// appendTrail adds message i's trailing blank rows to its lines and
// their plain mirror, and returns their count for Entry.Trail.
func (m *Model) appendTrail(i int, lines []string, plain []plainLine, cs cacheStyles) ([]string, []plainLine, int) {
	n := m.trailRows(i)
	for range n {
		lines = append(lines, cs.spacerLines...)
		plain = append(plain, plainLine{Text: "", Bytes: []int{0}})
	}
	return lines, plain, n * len(cs.spacerLines)
}

// groupingChanged reports whether message i's kept entry e was drawn
// with other neighbours than it has now.
func (m *Model) groupingChanged(i int, e entrylist.Entry, p EntryPayload) bool {
	return e.Trail != m.trailRows(i) || p.Continuation != m.continues(i)
}

// DayDivider is the day divider row, centred, and the blank row under
// it.
func DayDivider(width int, label string) string {
	return dividerStyle(width).Foreground(styles.TextMuted).Render("── "+label+" ──") + "\n" + blankRow(width)
}

// NewMessagesDivider is the new-messages line and the blank row under
// it.
func NewMessagesDivider(width int) string {
	return NewMessagesLine(width) + "\n" + blankRow(width)
}

func dividerStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().Background(styles.Background).Bold(true).Width(width).Align(lipgloss.Center)
}

// BlankRows is n full-width blank rows.
func BlankRows(width, n int) []string {
	rows := make([]string, n)
	row := blankRow(width)
	for i := range rows {
		rows[i] = row
	}
	return rows
}
