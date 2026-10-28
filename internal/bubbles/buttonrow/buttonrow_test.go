package buttonrow

import (
	"fmt"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestLayoutOneRow(t *testing.T) {
	t.Parallel()
	got := Layout([]string{"[a]", "[bb]"}, 20, 2)
	want := []Placed{
		{Index: 0, Row: 0, Col: 0, Text: "[a]", Width: 3},
		{Index: 1, Row: 0, Col: 5, Text: "[bb]", Width: 4},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Layout = %v, want %v", got, want)
	}
}

func TestLayoutWraps(t *testing.T) {
	t.Parallel()
	got := Layout([]string{"[aaaa]", "[bbbb]", "[cc]"}, 14, 2)
	// [aaaa]  [bbbb] is 14 wide: fits. [cc] would make it 20: wraps.
	if got[1].Row != 0 || got[1].Col != 8 || got[2].Row != 1 || got[2].Col != 0 {
		t.Errorf("Layout = %v", got)
	}
}

func TestLayoutShortensOverWideLabel(t *testing.T) {
	t.Parallel()
	got := Layout([]string{"[ Don't Show Again ]"}, 12, 2)
	if got[0].Width != 12 || lipgloss.Width(got[0].Text) != 12 {
		t.Errorf("width = %d (%q), want 12", got[0].Width, got[0].Text)
	}
	if r := []rune(got[0].Text); r[len(r)-1] != '…' {
		t.Errorf("want an ellipsis, got %q", got[0].Text)
	}
}

func TestRowsJoinsWithGap(t *testing.T) {
	t.Parallel()
	placed := Layout([]string{"[aaaa]", "[bbbb]", "[cc]"}, 14, 2)
	got := Rows(placed, 2)
	want := []string{"[aaaa]  [bbbb]", "[cc]"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Rows = %q, want %q", got, want)
	}
}

func TestRowsFuncRendersEachLabel(t *testing.T) {
	t.Parallel()
	placed := Layout([]string{"a", "b", "c"}, 3, 1)
	got := RowsFunc(placed, "_", func(p Placed) string { return "<" + p.Text + ">" })
	want := []string{"<a>_<b>", "<c>"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("RowsFunc = %q, want %q", got, want)
	}
}

func TestLayoutEmpty(t *testing.T) {
	t.Parallel()
	if got := Rows(Layout(nil, 10, 2), 2); len(got) != 0 {
		t.Errorf("Rows(nil) = %q, want none", got)
	}
}
