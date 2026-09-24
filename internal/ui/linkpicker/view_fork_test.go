package linkpicker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const multiSelectFooter = "space mark  a all  enter open  esc/q close"

func items4() []Item {
	return []Item{
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", Display: "#general · matt: deploy is out", InApp: true},
		{URL: "https://myteam.slack.com/archives/C2/p1700000000000002", Display: "#incidents · ana: paging db", InApp: true},
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000003", Display: "#general · matt: rollback plan", InApp: true},
		{URL: "https://myteam.slack.com/archives/C3/p1700000000000004", Display: "#random · lunch?", InApp: true},
	}
}

func TestRenderBox_MultiSelect(t *testing.T) {
	m := New()
	m.Open("Open link in herdr tab", items4())
	m.SetMultiSelect(true)
	m.ToggleMark()
	m.HandleKey("j")
	m.HandleKey("j")
	m.ToggleMark()
	out := ansi.Strip(m.renderBox(140))

	lines := strings.Split(out, "\n")
	wantRows := []string{
		" [x] #general · matt: deploy is out",
		" [ ] #incidents · ana: paging db",
		"▌[x] #general · matt: rollback plan",
		" [ ] #random · lunch?",
	}
	for _, want := range wantRows {
		if !strings.Contains(out, want) {
			t.Errorf("rendered box missing row %q", want)
		}
	}
	var titleLine string
	for _, l := range lines {
		if strings.Contains(l, "Open link in herdr tab") {
			titleLine = l
		}
	}
	// Right-aligned: the counter ends where the rows' "[slk]" badge ends.
	if !strings.HasSuffix(strings.TrimRight(titleLine, " │"), "2 marked") {
		t.Errorf("title line = %q, want a trailing \"2 marked\" counter", titleLine)
	}
	if !strings.Contains(out, multiSelectFooter) {
		t.Error("rendered box missing the multi-select footer")
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != ansi.StringWidth(lines[0]) {
			t.Errorf("line %q is %d cells wide, want %d", l, w, ansi.StringWidth(lines[0]))
		}
	}
}

// A footer wider than the box wraps, which splits it across a border
// and a newline: finding it whole means it stayed on one line.
func TestRenderBox_MultiSelect_FooterFitsEightyColumns(t *testing.T) {
	m := New()
	m.Open("Open link in herdr tab", items4())
	m.SetMultiSelect(true)
	if out := ansi.Strip(m.renderBox(80)); !strings.Contains(out, multiSelectFooter) {
		t.Errorf("multi-select footer wrapped at terminal width 80:\n%s", out)
	}
}

func TestRenderBox_MultiSelect_NoCounterAtZeroMarks(t *testing.T) {
	m := New()
	m.Open("Open link in herdr tab", items4())
	m.SetMultiSelect(true)
	if out := ansi.Strip(m.renderBox(140)); strings.Contains(out, "marked") {
		t.Error("counter shown with nothing marked")
	}
}

func TestRenderBox_NotMultiSelect_Unchanged(t *testing.T) {
	m := New()
	m.Open("Open link", items4())
	out := ansi.Strip(m.renderBox(140))
	for _, absent := range []string{"[ ]", "[x]", "marked", "space mark"} {
		if strings.Contains(out, absent) {
			t.Errorf("non-multi-select render contains %q", absent)
		}
	}
	if !strings.Contains(out, "▌#general · matt: deploy is out") {
		t.Error("cursor row lost its indicator-then-text layout")
	}
	if !strings.Contains(out, "j/k move   enter select   esc/q close") {
		t.Error("footer changed")
	}
}
