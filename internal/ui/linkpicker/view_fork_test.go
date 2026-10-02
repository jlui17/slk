package linkpicker

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const multiSelectFooter = "/ filter  space mark  a all  enter open  esc close"

// items4 is four permalink rows as the app has them once their previews
// have landed.
func items4() []Item {
	return []Item{
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", Display: "deploy is out", Side: "#general · matt", InApp: true},
		{URL: "https://myteam.slack.com/archives/C2/p1700000000000002", Display: "paging db", Side: "#incidents · ana", InApp: true},
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000003", Display: "rollback plan", Side: "#general · matt", InApp: true},
		{URL: "https://myteam.slack.com/archives/C3/p1700000000000004", Display: "lunch?", Side: "#random", InApp: true},
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
		" [x] deploy is out ",
		" [ ] paging db ",
		"▌[x] rollback plan ",
		" [ ] lunch? ",
		"  #incidents · ana ",
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
	if !strings.Contains(out, "▌deploy is out ") {
		t.Error("cursor row lost its indicator-then-text layout")
	}
	if !strings.Contains(out, "/ filter  j/k move  enter select  esc close") {
		t.Error("footer changed")
	}
}

// Rows without a Side keep their look: a file row draws its size right
// after its name, the box is as wide as its rows need, and beside
// permalink rows a plain URL runs across the Side column to the edge.
func TestRenderBox_RowsWithoutSideUnchanged(t *testing.T) {
	m := New()
	m.Open("Download file", []Item{
		{Label: "notes.txt", Detail: "12 KB"},
		{Label: "plan.pdf", Detail: "3 MB"},
	})
	out := ansi.Strip(m.renderBox(140))
	if !strings.Contains(out, "▌notes.txt  12 KB ") || ansi.StringWidth(strings.Split(out, "\n")[0]) != 80 {
		t.Errorf("files picker changed:\n%s", out)
	}

	long := "https://docs.example/" + strings.Repeat("a-long-path/", 6) + "end" // 96 cells: wider than the row left of the Side column
	m.Open("Open link", []Item{
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", Display: "Today", Side: "#general", InApp: true},
		{URL: long},
	})
	out = ansi.Strip(m.renderBox(140))
	if !strings.Contains(out, " "+long+" ") {
		t.Errorf("a plain URL row should run across the Side column:\n%s", out)
	}
}

// An emoji with a variation selector (❤️) or a keycap (1️⃣) is two cells
// to lipgloss and was one to the cut, so a cut string came out wider
// than its column: the Side column's padding went negative and View
// panicked, and a preview pushed the box wider. At every width the box
// draws, and every line of it is the box's width.
func TestRenderBox_EmojiWiderThanTheCutCounted(t *testing.T) {
	sets := map[string][]Item{
		"a sender": {
			{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", Display: "Today", Side: "#eng · Dana ❤️ Whitfield-Jones", InApp: true},
		},
		"a preview, a label, a channel, a detail": {
			{URL: "https://myteam.slack.com/archives/C1/p1700000000000002", Label: "❤️ 1️⃣ 2️⃣ 3️⃣ release notes", Display: "1️⃣ ❤️ 2️⃣ ❤️ 3️⃣ ❤️ " + strings.Repeat("deploy is out ", 12), Side: "#❤️1️⃣2️⃣3️⃣4️⃣5️⃣6️⃣7️⃣8️⃣9️⃣-releases · Dana", InApp: true},
			{URL: "https://other.slack.com/archives/C1/p1700000000000003", Display: "Today", Side: "other.slack.com " + strings.Repeat("❤️", 20)},
			{Label: "❤️.txt", Detail: strings.Repeat("1️⃣", 80)},
		},
	}
	for name, items := range sets {
		for _, multi := range []bool{false, true} {
			for width := 60; width <= 200; width++ {
				m := New()
				m.Open("Open link", items)
				m.SetMultiSelect(multi)
				lines := strings.Split(ansi.Strip(m.renderBox(width)), "\n")
				for _, l := range lines {
					if lipgloss.Width(l) != lipgloss.Width(lines[0]) || lipgloss.Width(l) > width-2 {
						t.Fatalf("%s, width %d, multi-select %v: line is %d cells, the box %d:\n%s", name, width, multi, lipgloss.Width(l), lipgloss.Width(lines[0]), l)
					}
				}
			}
		}
	}
}

// A row with a Side but no [slk] badge, a link to another workspace,
// keeps the badge's cells empty: its column starts where its neighbors'
// does.
func TestRenderBox_SideColumnAlignedWithoutBadge(t *testing.T) {
	m := New()
	m.Open("Open link", []Item{
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", Display: "Today", Side: "#general", InApp: true},
		{URL: "https://other.slack.com/archives/C1/p1700000000000002", Display: "Today", Side: "other.slack.com"},
	})
	lines := strings.Split(ansi.Strip(m.renderBox(120)), "\n")
	at := func(sub string) int {
		for _, l := range lines {
			if before, _, found := strings.Cut(l, sub); found {
				return ansi.StringWidth(before)
			}
		}
		return -1
	}
	if at("#general") < 0 || at("#general") != at("other.slack.com") {
		t.Errorf("columns start at cells %d and %d, want the same:\n%s", at("#general"), at("other.slack.com"), strings.Join(lines, "\n"))
	}
}
