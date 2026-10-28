package chrome

import (
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/scrollbar"
)

func TestWindow(t *testing.T) {
	cases := []struct {
		name                     string
		selected, total, maxRows int
		start, end               int
	}{
		{"empty list", 0, 0, 10, 0, 0},
		{"fits", 0, 5, 10, 0, 5},
		{"fits, selection at the end", 4, 5, 10, 0, 5},
		{"overflows, selection at the top", 0, 20, 10, 0, 10},
		{"last row of the first window", 9, 20, 10, 0, 10},
		{"one past the first window scrolls by one", 10, 20, 10, 1, 11},
		{"selection at the end", 19, 20, 10, 10, 20},
		{"no rows allowed", 5, 20, 0, 0, 0},
		{"negative rows allowed", 5, 20, -3, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, e := Window(c.selected, c.total, c.maxRows)
			if s != c.start || e != c.end {
				t.Errorf("Window(%d, %d, %d) = [%d, %d), want [%d, %d)",
					c.selected, c.total, c.maxRows, s, e, c.start, c.end)
			}
		})
	}
}

func TestMarker(t *testing.T) {
	s := testStyles()
	if got := xansi.Strip(Marker(s, true)); got != "▌" {
		t.Errorf("selected marker = %q, want ▌", got)
	}
	if got := Marker(s, false); got != " " {
		t.Errorf("unselected marker = %q, want a plain space", got)
	}
}

func TestList_ContentWidth(t *testing.T) {
	if got := (List{Start: 0, End: 5, Total: 5}).ContentWidth(20); got != 19 {
		t.Errorf("non-scrolling content width = %d, want 19 (marker only)", got)
	}
	if got := (List{Start: 0, End: 5, Total: 9}).ContentWidth(20); got != 18 {
		t.Errorf("scrolling content width = %d, want 18 (marker and gutter)", got)
	}
}

func TestList_GutterFollowsThumb(t *testing.T) {
	s := testStyles()
	l := List{Start: 5, End: 10, Total: 20}
	ts, te := scrollbar.Thumb(20, 5, 5)
	for i := l.Start; i < l.End; i++ {
		want := "│"
		if rel := i - l.Start; rel >= ts && rel < te {
			want = "█"
		}
		if got := xansi.Strip(l.Gutter(s, i)); got != want {
			t.Errorf("Gutter(row %d) = %q, want %q", i, got, want)
		}
	}
	if got := (List{Start: 0, End: 3, Total: 3}).Gutter(s, 1); got != "" {
		t.Errorf("a list that does not scroll drew gutter %q", got)
	}
}

func TestList_Rows(t *testing.T) {
	s := testStyles()
	cases := []struct {
		name     string
		l        List
		selected int
		want     []string
	}{
		{"no scroll", List{Start: 0, End: 2, Total: 2}, 1, []string{" aa", "▌bb"}},
		// Thumb(5, 3, 2) is [1, 2): the second visible row.
		{"scrolled; selected indexes the whole list", List{Start: 3, End: 5, Total: 5}, 4, []string{" aa│", "▌bb█"}},
		{"selection outside the window", List{Start: 0, End: 2, Total: 2}, 9, []string{" aa", " bb"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.l.Rows(s, c.selected, []string{"aa", "bb"})
			for i := range c.want {
				if g := xansi.Strip(got[i]); g != c.want[i] {
					t.Errorf("row %d = %q, want %q", i, g, c.want[i])
				}
			}
		})
	}
}
