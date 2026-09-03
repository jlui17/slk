package messages

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/styles"
)

const quoteBar = "┃"

func strippedRows(out string) []string {
	return strings.Split(ansi.Strip(out), "\n")
}

func requireBarredRows(t *testing.T, name string, lines []string, minRows int) {
	t.Helper()
	if len(lines) < minRows {
		t.Fatalf("%s: expected at least %d barred rows, got %d: %q", name, minRows, len(lines), lines)
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, quoteBar) {
			t.Errorf("%s: row %d lacks the quote bar: %q", name, i, l)
		}
	}
}

// The hosts WordWrap the rendered body again; that pass must leave the
// renderer's rows alone.
func hostPasses(out string, width int) map[string]string {
	return map[string]string{"rendered": out, "rendered+WordWrap": WordWrap(out, width)}
}

func TestBlockquote_WrapsInsideBar(t *testing.T) {
	const width = 30
	out := RenderSlackMarkdownWith("> "+strings.Repeat("word ", 20), RenderSlackMarkdownOpts{Width: width})
	for name, pass := range hostPasses(out, width) {
		lines := strippedRows(pass)
		requireBarredRows(t, name, lines, 3)
		requireRowsWithin(t, name, lines, width)
	}
}

func TestBlockquote_ConsecutiveLinesShareBar(t *testing.T) {
	in := "> first\n&gt; second\n> \n> fourth\nplain after"
	lines := strippedRows(RenderSlackMarkdownWith(in, RenderSlackMarkdownOpts{Width: 40}))
	if len(lines) != 5 {
		t.Fatalf("expected 5 rows, got %d: %q", len(lines), lines)
	}
	requireBarredRows(t, "quote rows", lines[:4], 4)
	if strings.Contains(lines[4], quoteBar) || !strings.Contains(lines[4], "plain after") {
		t.Errorf("row 4 should be unquoted plain text, got %q", lines[4])
	}
	if !strings.Contains(lines[1], "second") || !strings.Contains(lines[3], "fourth") {
		t.Errorf("quote body lost text: %q", lines)
	}
}

func TestBlockquote_NoWidthLeavesLineIntact(t *testing.T) {
	out := RenderSlackMarkdownWith("> "+strings.Repeat("word ", 30), RenderSlackMarkdownOpts{})
	if lines := strippedRows(out); len(lines) != 1 || !strings.HasPrefix(lines[0], quoteBar) {
		t.Errorf("expected one barred row, got %q", lines)
	}
}

func TestBlockquote_BlockKitTextWrapsInsideBar(t *testing.T) {
	m := New(nil, "general")
	ctx := m.blockkitContext(MessageItem{TS: "1.0"}, nil, nil)
	out := ctx.RenderTextForWidth("&gt; "+strings.Repeat("word ", 20), nil, 30)
	requireBarredRows(t, "blockkit text", strippedRows(out), 3)
}

func searchHighlightSGRForTest(t *testing.T) (start, end string) {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	start, end, ok := SearchHighlightSGR()
	if !ok {
		t.Fatal("SearchHighlightSGR returned !ok")
	}
	return start, end
}

// A 60-char URL inside a quote at Width 30 hard-breaks into rows of 28:
// "https://example.com/01234567" | "89abcdefghijklmnopqrstuvwxyz" | "ABCD".
const hardBrokenQuoteURL = "https://example.com/0123456789abcdefghijklmnopqrstuvwxyzABCD"

func TestBlockquote_TermSpanningHardBreakHighlightsEveryRow(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	term := "https://example.com/0123456789abc"
	out := RenderSlackMarkdownWith("&gt; "+hardBrokenQuoteURL, RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{term}})
	rows := strings.Split(out, "\n")
	requireBarredRows(t, "highlighted quote", strippedRows(out), 3)
	if !strings.Contains(rows[0], hlStart+"https://example.com/01234567") {
		t.Errorf("row 0 does not open the highlight at the term start: %q", rows[0])
	}
	_, tail, found := strings.Cut(rows[1], hlStart+"89abc"+hlEnd)
	if !found {
		t.Fatalf("row 1 does not re-open the highlight for the term's continuation: %q", rows[1])
	}
	beforeNextVisible, _, _ := strings.Cut(tail, "defg")
	quoteFg := fgANSIFor(blockquoteStyle().GetForeground())
	if strings.LastIndex(beforeNextVisible, quoteFg) < strings.LastIndex(beforeNextVisible, FgANSI()) {
		t.Errorf("row 1 does not return to the quote fg after the highlight close: %q", rows[1])
	}
}

// "https://example.com/1234567/" is exactly the 28-column inner width, so
// the term starts on the hard-break column.
func TestBlockquote_TermAtHardBreakColumnHighlights(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("&gt; https://example.com/1234567/deploy-x", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"deploy"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("term at the hard-break column not highlighted: %q", out)
	}
}

func TestBlockquote_SecondFragmentIsNotAWordStart(t *testing.T) {
	hlStart, _ := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("&gt; "+hardBrokenQuoteURL, RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"89abc"}})
	if strings.Contains(out, hlStart) {
		t.Errorf("mid-word term highlighted at a hard-break row start: %q", out)
	}
}

func TestBlockquote_WordBoundaryWrapStillHighlights(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("> "+strings.Repeat("word ", 10)+"deploy done", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"deploy"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("term after a word-boundary wrap not highlighted: %q", out)
	}
}

func TestRenderSlackMarkdownWith_SearchTermsHighlightPlainLines(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("deploy went fine &amp; dandy", RenderSlackMarkdownOpts{SearchTerms: []string{"deploy", "&"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("plain line term not highlighted: %q", out)
	}
	if !strings.Contains(out, hlStart+"&"+hlEnd) {
		t.Errorf("entity-decoded text not highlighted (highlight must run after decode): %q", out)
	}
}

func TestRenderMessagePlain_HighlightsSearchTerms(t *testing.T) {
	hlStart, _ := searchHighlightSGRForTest(t)
	m := New(nil, "general")
	m.SetSearchTerms([]string{"deploy"})
	content, _, _, _, _ := m.renderMessagePlain(MessageItem{TS: "1.0", UserName: "u", Text: "deploy went fine"}, 80, "", nil, nil, false, nil)
	if !strings.Contains(content, hlStart+"deploy") {
		t.Errorf("messages pane body lost search highlighting: %q", content)
	}
}

func TestBlockKitText_HighlightsSearchTerms(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	m := New(nil, "general")
	m.SetSearchTerms([]string{"deploy"})
	ctx := m.blockkitContext(MessageItem{TS: "1.0"}, nil, nil)
	if out := ctx.RenderTextForWidth("deploy went fine", nil, 30); !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("block kit text not highlighted: %q", out)
	}
}

func requireRowsWithin(t *testing.T, name string, rows []string, width int) {
	t.Helper()
	for i, r := range rows {
		if w := lipgloss.Width(r); w > width {
			t.Errorf("%s: row %d is %d wide, limit %d: %q", name, i, w, width, r)
		}
	}
}

func nonBlankRows(out string) []string {
	var rows []string
	for _, r := range strippedRows(out) {
		if strings.TrimSpace(r) != "" {
			rows = append(rows, r)
		}
	}
	return rows
}

func TestCodeBlock_WrapsInsideBox(t *testing.T) {
	const width = 30
	code := "func f() {\n    if a {\n        return 1234567890 1234567890 1234567890\n    }\n}"
	out := RenderSlackMarkdownWith("```\n"+code+"\n```", RenderSlackMarkdownOpts{Width: width})
	for name, pass := range hostPasses(out, width) {
		rows := nonBlankRows(pass)
		if len(rows) < 6 {
			t.Fatalf("%s: expected the long code line to split, got %d rows: %q", name, len(rows), rows)
		}
		requireRowsWithin(t, name, rows, width)
		boxWidth := lipgloss.Width(rows[0])
		for i, r := range rows {
			if lipgloss.Width(r) != boxWidth {
				t.Errorf("%s: row %d is %d wide, box is %d: %q", name, i, lipgloss.Width(r), boxWidth, r)
			}
		}
		if !strings.HasPrefix(rows[1], "     if a {") {
			t.Errorf("%s: code indentation lost: %q", name, rows[1])
		}
		if !strings.HasPrefix(rows[2], "         return") {
			t.Errorf("%s: deeper indentation lost: %q", name, rows[2])
		}
	}
}

func TestCodeBlock_TabsCountAsFourCells(t *testing.T) {
	const width = 20
	out := RenderSlackMarkdownWith("```\nf()\n\t\t"+strings.Repeat("x", 30)+"\n```", RenderSlackMarkdownOpts{Width: width})
	rows := nonBlankRows(out)
	requireRowsWithin(t, "tabbed code", rows, width)
	if len(rows) < 3 || !strings.HasPrefix(rows[1], strings.Repeat(" ", 9)+"x") {
		t.Errorf("expected the tabbed line to keep 8 cells of indent inside the box and split, got %q", rows)
	}
}

func TestCodeBlock_NoWidthLeavesRowsIntact(t *testing.T) {
	out := RenderSlackMarkdownWith("```\n"+strings.Repeat("x", 80)+"\n```", RenderSlackMarkdownOpts{})
	if rows := nonBlankRows(out); len(rows) != 1 {
		t.Errorf("expected one code row, got %q", rows)
	}
}

func requireHangingIndent(t *testing.T, name string, rows []string, marker, indent string, width int) {
	t.Helper()
	if len(rows) < 2 {
		t.Fatalf("%s: expected the item to wrap, got %q", name, rows)
	}
	requireRowsWithin(t, name, rows, width)
	if !strings.HasPrefix(rows[0], marker) {
		t.Errorf("%s: first row lost its marker: %q", name, rows[0])
	}
	for i, r := range rows[1:] {
		if !strings.HasPrefix(r, indent) || strings.HasPrefix(r, indent+" ") {
			t.Errorf("%s: continuation row %d lacks a %d-cell hanging indent: %q", name, i+1, len(indent), r)
		}
	}
}

func TestListItem_HangingIndent(t *testing.T) {
	const width = 30
	body := strings.Repeat("item ", 10)
	cases := map[string]struct{ marker, indent string }{
		"bullet":        {"• ", "  "},
		"numbered":      {"12. ", "    "},
		"nested bullet": {"  • ", "    "},
	}
	for name, tc := range cases {
		out := RenderSlackMarkdownWith(tc.marker+body, RenderSlackMarkdownOpts{Width: width})
		for pass, s := range hostPasses(out, width) {
			requireHangingIndent(t, name+"/"+pass, strippedRows(s), tc.marker, tc.indent, width)
		}
	}
}

func TestListItem_NoWidthLeavesLineIntact(t *testing.T) {
	out := RenderSlackMarkdownWith("• "+strings.Repeat("item ", 30), RenderSlackMarkdownOpts{})
	if rows := strippedRows(out); len(rows) != 1 {
		t.Errorf("expected one row, got %q", rows)
	}
}

// listToMrkdwn is the producer of the marker shape the renderer indents
// under; a nested rich_text list must land on the hanging-indent path.
func TestListItem_RichTextListWrapsUnderMarker(t *testing.T) {
	list := &slack.RichTextList{
		Type: slack.RTEList, Style: slack.RTEListOrdered, Indent: 1,
		Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: strings.Repeat("item ", 10)},
			}},
		},
	}
	md := blockkit.RichTextToMrkdwn(blockkit.RichTextBlock{Elements: []slack.RichTextElement{list}})
	m := New(nil, "general")
	out := m.blockkitContext(MessageItem{TS: "1.0"}, nil, nil).RenderTextForWidth(md, nil, 30)
	requireHangingIndent(t, "rich_text list", strippedRows(out), "  1. ", "     ", 30)
}

func TestWordWrap_KeepsFittingRowsVerbatim(t *testing.T) {
	in := "  two  spaces \n    indented"
	if got := WordWrap(in, 20); got != in {
		t.Errorf("WordWrap rewrote rows that already fit:\n got %q\nwant %q", got, in)
	}
}

func TestWordWrap_TabsCountAsPainted(t *testing.T) {
	const limit = 18
	for _, in := range []string{"aaaa\tbbbb\tcccc\tdd", "> a\tb\tc\tddddddd"} {
		out := lipgloss.NewStyle().Render(WordWrap(RenderSlackMarkdownWith(in, RenderSlackMarkdownOpts{Width: limit}), limit))
		requireRowsWithin(t, in, strippedRows(out), limit)
	}
}

func TestCodeBlock_RowsAreNotListItems(t *testing.T) {
	out := RenderSlackMarkdownWith("```\n• "+strings.Repeat("x", 40)+"\n```", RenderSlackMarkdownOpts{Width: 30})
	rows := nonBlankRows(out)
	if len(rows) < 2 || !strings.HasPrefix(rows[1], " xx") {
		t.Errorf("expected the continuation row to stay a code row, got %q", rows)
	}
}

func TestCodeBlock_DecodesEntitiesBeforeWrapping(t *testing.T) {
	const width = 30
	out := RenderSlackMarkdownWith("```\n"+strings.Repeat("x", 27)+"&gt; b\n```", RenderSlackMarkdownOpts{Width: width})
	rows := nonBlankRows(out)
	if len(rows) != 2 || strings.TrimSpace(rows[0])+strings.TrimSpace(rows[1]) != strings.Repeat("x", 27)+">b" {
		t.Fatalf("expected the entity to be decoded before the break, got %q", rows)
	}
	if lipgloss.Width(rows[0]) != lipgloss.Width(rows[1]) {
		t.Errorf("box edge is ragged: %q", rows)
	}
}

func TestListItem_HighlightsSearchTerms(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("• "+strings.Repeat("item ", 8)+"deploy done", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"deploy"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("list item term not highlighted: %q", out)
	}
}

func TestCodeBlock_RowsHighlightSearchTerms(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("```\nfunc deploy() {}\n```", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"deploy"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("code row term not highlighted: %q", out)
	}
}
