package linkpicker

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// tableItems is 46 permalink rows as the app opens them for a table of
// numbered links: two per table row, labels padded to one column.
func tableItems() []Item {
	items := make([]Item, 46)
	for i := range items {
		url := fmt.Sprintf("https://myteam.slack.com/archives/C1/p17000000000000%02d", i)
		items[i] = Item{
			URL:     url,
			Label:   fmt.Sprintf("%-28s", fmt.Sprintf("job %d (Q%d) · %d", i/2+1, i/2+3, i%2+1)),
			Display: "Today",
			Side:    "#eng-ops",
			InApp:   true,
		}
	}
	return items
}

func openTable(t *testing.T) *Model {
	t.Helper()
	m := New()
	m.Open("Open link in herdr tab", tableItems())
	m.SetMultiSelect(true)
	m.SetTermHeight(24)
	return m
}

func keys(m *Model, keys ...string) {
	for _, k := range keys {
		m.HandleKey(k)
	}
}

func boxLines(t *testing.T, m *Model) []string {
	t.Helper()
	out := ansi.Strip(m.renderBox(80))
	t.Logf("\n%s", out)
	lines := strings.Split(out, "\n")
	if len(lines) > 24 {
		t.Errorf("box is %d lines tall on a 24-line terminal", len(lines))
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 78 {
			t.Errorf("line %q is %d cells wide, want 78", l, w)
		}
	}
	return lines
}

func lineWith(lines []string, sub string) string {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func TestWindow_TopOfList(t *testing.T) {
	lines := boxLines(t, openTable(t))
	if !strings.Contains(lineWith(lines, "Open link"), "1–14 of 46") {
		t.Error("title line missing the position 1–14 of 46")
	}
	if lineWith(lines, "job 7 (Q9) · 2") == "" || lineWith(lines, "job 8 (Q10) · 1") != "" {
		t.Error("window is not rows 1 to 14")
	}
	// The label column: every row's Display starts at one cell.
	displayAt := func(row string) int {
		before, _, _ := strings.Cut(lineWith(lines, row), "#eng-ops · Today")
		return ansi.StringWidth(before)
	}
	if at := displayAt("job 1 (Q3) · 1"); at != 37 || displayAt("job 7 (Q9) · 2") != at {
		t.Error("label column is not aligned")
	}
}

func TestWindow_FollowsTheHighlight(t *testing.T) {
	m := openTable(t)
	for i := 0; i < 30; i++ {
		m.HandleKey("j")
		m.window() // as a draw after every key
	}
	lines := boxLines(t, m)
	if !strings.Contains(lineWith(lines, "job 16 (Q18) · 1"), "▌") {
		t.Error("row 31 is not drawn highlighted after 30 downs")
	}
	if !strings.Contains(lineWith(lines, "Open link"), "18–31 of 46") {
		t.Error("title line missing the position 18–31 of 46")
	}
	// Up moves the highlight inside the window; the window scrolls only
	// at its edge.
	keys(m, "k", "k")
	if !strings.Contains(lineWith(boxLines(t, m), "Open link"), "18–31 of 46") {
		t.Error("window scrolled before the highlight reached its edge")
	}
}

func TestWindow_AllRowsFit_NoPosition(t *testing.T) {
	m := New()
	m.Open("Open link", items4())
	m.SetTermHeight(24)
	if out := ansi.Strip(m.renderBox(80)); strings.Contains(out, " of ") {
		t.Errorf("position shown though every row fits:\n%s", out)
	}
}

func TestFilter(t *testing.T) {
	m := openTable(t)
	keys(m, "/", "Q", "1", "6", ")")
	lines := boxLines(t, m)
	if lineWith(lines, "/Q16)█") == "" {
		t.Error("filter line missing")
	}
	if !strings.Contains(lineWith(lines, "Open link"), "2 matches") {
		t.Error("title line missing the match count")
	}
	if !strings.Contains(lineWith(lines, "job 14 (Q16) · 1"), "▌") || lineWith(lines, "job 14 (Q16) · 2") == "" {
		t.Error("filter did not narrow to row 14's links with the first highlighted")
	}
	if lineWith(lines, "job 1 (Q3)") != "" {
		t.Error("filter kept a row that does not match")
	}

	// tab hands the keys back to the list: a marks what the filter shows.
	keys(m, "tab")
	m.ToggleMarkAll()
	keys(m, "esc")
	if !m.IsVisible() || len(m.shown()) != 46 {
		t.Fatal("first esc should clear the filter and keep the picker open")
	}
	if got := markedURLs(m); !slices.Equal(got, []string{tableItems()[26].URL, tableItems()[27].URL}) {
		t.Errorf("Marked after clearing the filter = %v, want row 14's two links", got)
	}
	if m.Selected() != 26 {
		t.Errorf("Selected = %d, want 26: clearing the filter keeps the highlight", m.Selected())
	}
	keys(m, "esc")
	if m.IsVisible() {
		t.Error("second esc should close the picker")
	}
}

func TestFilter_TypedKeysAreText(t *testing.T) {
	m := openTable(t)
	keys(m, "/", "q", "j", "a", "space", "backspace", "backspace")
	if !m.IsVisible() || m.filter != "qj" {
		t.Errorf("visible=%v filter=%q, want the picker open on filter \"qj\"", m.IsVisible(), m.filter)
	}
	if _, chosen := m.HandleKey("enter"); chosen {
		t.Error("enter chose a row though the filter shows none")
	}
	lines := boxLines(t, m)
	if lineWith(lines, "no matching rows") == "" {
		t.Error("no-match hint missing")
	}
}

func TestFilter_EnterOpensTheHighlightedMatch(t *testing.T) {
	m := openTable(t)
	keys(m, "/", "q", "1", "8", "down")
	item, chosen := m.HandleKey("enter")
	if !chosen || item.Index != 31 {
		t.Errorf("enter = %+v, %v; want row 31, the second link of job 16 (Q18)", item, chosen)
	}
}

// A row that shows its URL is filtered by it; a permalink behind a
// Display is not, or "0031" would match a row that shows no such text.
func TestFilter_ReadsWhatTheRowShows(t *testing.T) {
	m := New()
	m.Open("Open link", append(tableItems(), Item{URL: "https://docs.example/0031"}))
	keys(m, "/", "0", "0", "3", "1")
	if got := m.shown(); !slices.Equal(got, []int{46}) {
		t.Errorf("shown = %v, want only the row that shows its URL", got)
	}
}

// The filter reads FilterText and nothing else of a row that has one;
// a row without one is read as its whole text at Open, past where the
// box cuts it.
func TestFilter_ReadsFilterTextOrTheWholeRowAtOpen(t *testing.T) {
	m := New()
	m.Open("Open link", []Item{
		{URL: "https://a.example", Label: "row 1, Sam: rename th… · 1", FilterText: "row 1, Sam: rename the build cache · 1", Display: "Today", Side: "#eng-ops"},
		{URL: "https://b.example", Label: "notes.txt", Display: strings.Repeat("the nightly export ", 10) + "is stuck"},
	})
	keys(m, "/", "c", "a", "c", "h", "e")
	if got := m.shown(); !slices.Equal(got, []int{0}) {
		t.Errorf("shown = %v, want the row whose FilterText has \"cache\"", got)
	}
	keys(m, "esc", "/", "s", "t", "u", "c", "k")
	if got := m.shown(); !slices.Equal(got, []int{1}) {
		t.Errorf("shown = %v, want the row whose text ends \"stuck\", past where the box cuts it", got)
	}
	if out := ansi.Strip(m.renderBox(80)); strings.Contains(out, "is stuck") || strings.Contains(out, "build cache") {
		t.Errorf("the drawn rows should stay cut:\n%s", out)
	}
	keys(m, "esc", "/", "t", "o", "d", "a", "y")
	if got := m.shown(); len(got) != 0 {
		t.Errorf("shown = %v, want none: a row with FilterText is not read by its Display", got)
	}
}

// What the filter reads is fixed at Open: a preview that lands while a
// filter is on changes neither the rows it shows nor the highlight.
func TestFilter_PreviewLandingChangesNothing(t *testing.T) {
	m := openTable(t)
	keys(m, "/", "t", "o", "d", "a", "y", "down", "down")
	shown, selected := m.shown(), m.Selected()
	if len(shown) != 46 || selected != 2 {
		t.Fatalf("shown %d rows with row %d highlighted, want 46 and 2", len(shown), selected)
	}
	for i := range tableItems() {
		m.SetDisplay(i, "the nightly export is stuck")
		m.SetSide(i, "#eng-ops · dana")
	}
	if got := m.shown(); !slices.Equal(got, shown) || m.Selected() != selected {
		t.Errorf("after the previews landed: %d rows shown, row %d highlighted; want %d and %d", len(got), m.Selected(), len(shown), selected)
	}
	keys(m, "esc", "/", "s", "t", "u", "c", "k")
	if got := m.shown(); len(got) != 0 {
		t.Errorf("filter \"stuck\" shows %d rows, want none: preview text is not read", len(got))
	}
}

// Under a filter the title line counts matches: "1 match", "K matches"
// when they all fit, "N–M of K matches" when they do not, and nothing
// beside "no matching rows".
func TestTitleStatus_UnderAFilter(t *testing.T) {
	title := func(filter ...string) string {
		m := openTable(t)
		keys(m, append([]string{"/"}, filter...)...)
		line := lineWith(strings.Split(ansi.Strip(m.renderBox(80)), "\n"), "Open link")
		return strings.TrimSpace(strings.TrimPrefix(strings.Trim(line, "│ "), "Open link in herdr tab"))
	}
	for _, tt := range []struct {
		filter []string
		want   string
	}{
		{[]string{"Q", "1", "6", ")", "space", "·", "space", "1"}, "1 match"},
		{[]string{"Q", "1", "6", ")"}, "2 matches"},
		{[]string{"j", "o", "b", "space", "1"}, "1–14 of 22 matches"},
		{[]string{"z", "z"}, ""},
	} {
		if got := title(tt.filter...); got != tt.want {
			t.Errorf("filter %q: title status = %q, want %q", strings.Join(tt.filter, ""), got, tt.want)
		}
	}
}
