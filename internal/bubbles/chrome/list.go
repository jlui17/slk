package chrome

import "github.com/gammons/slk/internal/bubbles/scrollbar"

// Window returns the [start, end) rows of a list of total rows to show
// in at most maxRows rows so that selected is visible: the top of the
// list until selected passes the last row, then the window that ends at
// selected. It is the one list window every dialog uses.
func Window(selected, total, maxRows int) (start, end int) {
	n := min(maxRows, total)
	if n <= 0 {
		return 0, 0
	}
	if selected >= n {
		start = selected - n + 1
	}
	end = start + n
	if end > total {
		end = total
		start = max(end-n, 0)
	}
	return start, end
}

// Marker is the column in front of a list row: ▌ in s.Marker for the
// selected row, a plain space otherwise.
func Marker(s Styles, selected bool) string {
	if selected {
		return s.Marker.Render("▌")
	}
	return " "
}

// List is the visible window [Start, End) of a list of Total rows.
// A dialog renders each row's content ContentWidth wide; Rows (or
// Marker and Gutter, for rows that span several lines) adds the marker
// column and, when the list scrolls, the scrollbar gutter.
type List struct {
	Start, End, Total int
}

// Scrolls reports whether the list is longer than its window, which is
// when it gets a scrollbar.
func (l List) Scrolls() bool {
	return l.Total > l.End-l.Start
}

// ContentWidth is the width left for a row's content inside inner
// columns, after the marker column and, when the list scrolls, the
// gutter column.
func (l List) ContentWidth(inner int) int {
	w := inner - 1
	if l.Scrolls() {
		w--
	}
	return w
}

// Gutter is the scrollbar cell for row i (an index into the whole list,
// within [Start, End)): █ in s.Thumb on the thumb, │ in s.Track
// elsewhere, and "" when the list does not scroll. The thumb is
// scrollbar.Thumb in row units, so every line of a multi-line row gets
// the same cell.
func (l List) Gutter(s Styles, i int) string {
	if !l.Scrolls() {
		return ""
	}
	ts, te := scrollbar.Thumb(l.Total, l.Start, l.End-l.Start)
	if rel := i - l.Start; rel >= ts && rel < te {
		return s.Thumb.Render("█")
	}
	return s.Track.Render("│")
}

// Rows draws rows, the content of rows [Start, Start+len(rows)), each
// already ContentWidth wide: the marker, the content, the gutter.
// selected is an index into the whole list.
func (l List) Rows(s Styles, selected int, rows []string) []string {
	out := make([]string, len(rows))
	for k, r := range rows {
		i := l.Start + k
		out[k] = Marker(s, i == selected) + r + l.Gutter(s, i)
	}
	return out
}
