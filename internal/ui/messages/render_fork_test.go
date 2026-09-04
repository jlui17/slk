package messages

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/gammons/slk/internal/ui/syntax"
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
		code := codeRows(pass)
		if len(code) < 3 || code[1] != "    if a {" || !strings.HasPrefix(code[2], "        return") {
			t.Errorf("%s: code indentation lost: %q", name, code)
		}
	}
}

func TestCodeBlock_TabsCountAsFourCells(t *testing.T) {
	const width = 20
	out := RenderSlackMarkdownWith("```\nf()\n\t\t"+strings.Repeat("x", 30)+"\n```", RenderSlackMarkdownOpts{Width: width})
	rows := nonBlankRows(out)
	requireRowsWithin(t, "tabbed code", rows, width)
	requireCodeRows(t, "tabbed code", out, "f()", strings.Repeat(" ", 8)+strings.Repeat("x", 8), strings.Repeat("x", 16), strings.Repeat("x", 6))
}

func TestCodeBlock_NoWidthLeavesRowsIntact(t *testing.T) {
	out := RenderSlackMarkdownWith("```\n"+strings.Repeat("x", 80)+"\n```", RenderSlackMarkdownOpts{})
	requireCodeRows(t, "unbounded code", out, strings.Repeat("x", 80))
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
	requireCodeRows(t, "continuation row", out, "• "+strings.Repeat("x", 24), strings.Repeat("x", 16))
}

func TestCodeBlock_DecodesEntitiesBeforeWrapping(t *testing.T) {
	const width = 30
	out := RenderSlackMarkdownWith("```\n"+strings.Repeat("x", 27)+"&gt; b\n```", RenderSlackMarkdownOpts{Width: width})
	rows := nonBlankRows(out)
	code := codeRows(out)
	if len(code) != 2 || code[0]+code[1] != strings.Repeat("x", 27)+"> b" {
		t.Fatalf("expected the entity to be decoded before the break, got %q", rows)
	}
	for _, r := range rows {
		if lipgloss.Width(r) != lipgloss.Width(rows[0]) {
			t.Errorf("box edge is ragged: %q", rows)
		}
	}
}

func TestListItem_HighlightsSearchTerms(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("• "+strings.Repeat("item ", 8)+"deploy done", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"deploy"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("list item term not highlighted: %q", out)
	}
}

func TestCodeBlock_RowsAreLiteral(t *testing.T) {
	code := "Init(ctx, *Config) error   Init(ctx, *Config) error\n_x_ ~y~ `z` :smile:"
	out := RenderSlackMarkdownWith("```\n"+code+"\n```", RenderSlackMarkdownOpts{Width: 60})
	plain := ansi.Strip(out)
	for _, want := range strings.Split(code, "\n") {
		if !strings.Contains(plain, want) {
			t.Errorf("code row lost its literal text %q:\n%s", want, plain)
		}
	}
}

func TestCodeBlock_RowsHighlightSearchTerms(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("```\nfunc deploy() {}\n```", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"deploy"}})
	if !strings.Contains(out, hlStart+"deploy"+hlEnd) {
		t.Errorf("code row term not highlighted: %q", out)
	}
}

func withDarkTheme(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
}

func sgrPrefix(t *testing.T, style lipgloss.Style) string {
	t.Helper()
	probe := style.Render("x")
	end := strings.IndexByte(probe, 'm')
	if !strings.HasPrefix(probe, "\x1b[") || end < 0 {
		t.Fatalf("style emitted no SGR: %q", probe)
	}
	return probe[:end+1]
}

func TestBlockquote_InlineFormattingMatchesPlainLine(t *testing.T) {
	withDarkTheme(t)
	opts := RenderSlackMarkdownOpts{UserNames: map[string]string{"U1": "alice"}}
	cases := []struct {
		name, in string
		style    lipgloss.Style
	}{
		{"bold", "say *loud* now", boldStyle()},
		{"italic", "say _soft_ now", italicStyle()},
		{"strike", "say ~gone~ now", strikethroughStyle()},
		{"code", "run `ls` now", codeStyle()},
		{"link", "see <https://example.com|docs> now", linkStyle()},
		{"mention", "hi <@U1> now", mentionStyle()},
	}
	for _, tc := range cases {
		plain := RenderSlackMarkdownWith(tc.in, opts)
		quote := RenderSlackMarkdownWith("> "+tc.in, opts)
		if want, got := quoteBar+" "+ansi.Strip(plain), ansi.Strip(quote); got != want {
			t.Errorf("%s: quote text %q, want %q", tc.name, got, want)
		}
		if sgr := sgrPrefix(t, tc.style); !strings.Contains(quote, sgr) {
			t.Errorf("%s: quote lacks the span's SGR %q:\n%q", tc.name, sgr, quote)
		}
	}
}

func TestBlockquote_EmojiShortcodeConverts(t *testing.T) {
	if got := ansi.Strip(RenderSlackMarkdown("> :smile: there", nil, nil)); strings.Contains(got, ":smile:") {
		t.Errorf("emoji shortcode left raw inside quote: %q", got)
	}
}

func TestBlockquote_EscapedMarkupStaysLiteral(t *testing.T) {
	opts := RenderSlackMarkdownOpts{UserNames: map[string]string{"U1": "alice"}}
	got := ansi.Strip(RenderSlackMarkdownWith("> &lt;@U1&gt; and &lt;https://x|y&gt;", opts))
	if want := quoteBar + " <@U1> and <https://x|y>"; got != want {
		t.Errorf("escaped markup inside quote: got %q, want %q", got, want)
	}
}

func TestBlockquote_StyledSpanSurvivesWrap(t *testing.T) {
	withDarkTheme(t)
	const width = 30
	in := "> " + strings.Repeat("word ", 8) + "*loud* " + strings.Repeat("word ", 8)
	out := RenderSlackMarkdownWith(in, RenderSlackMarkdownOpts{Width: width})
	bold := sgrPrefix(t, boldStyle())
	for name, pass := range map[string]string{"rendered": out, "rendered+WordWrap": WordWrap(out, width)} {
		rows := strings.Split(pass, "\n")
		requireBarredRows(t, name, strippedRows(pass), 3)
		if !slices.ContainsFunc(rows, func(row string) bool {
			return strings.Contains(row, bold) && strings.Contains(ansi.Strip(row), "loud")
		}) {
			t.Errorf("%s: no barred row carries the bold span:\n%q", name, rows)
		}
	}
}

func TestBlockquote_TailStaysMutedAfterSpan(t *testing.T) {
	withDarkTheme(t)
	muted, primary := fgANSIFor(blockquoteStyle().GetForeground()), FgANSI()
	if muted == primary || muted == "" {
		t.Fatalf("theme must distinguish muted %q from primary %q", muted, primary)
	}
	out := RenderSlackMarkdown("> before *loud* after", nil, nil)
	_, tail, _ := strings.Cut(out, "loud")
	tail, _, _ = strings.Cut(tail, "after")
	if !strings.Contains(tail, muted) || strings.LastIndex(tail, muted) < strings.LastIndex(tail, primary) {
		t.Errorf("quote text after a span is not muted; escapes before %q: %q", "after", tail)
	}
}

func TestBlockquote_RichTextQuoteRendersInline(t *testing.T) {
	withDarkTheme(t)
	rt := blockkit.RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextQuote{Type: slack.RTEQuote, Elements: []slack.RichTextSectionElement{
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "say "},
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "loud", Style: &slack.RichTextSectionTextStyle{Bold: true}},
		}},
	}}
	m := New(nil, "general")
	r := blockkit.Render([]blockkit.Block{rt}, m.blockkitContext(MessageItem{TS: "1.0"}, nil, nil), 40)
	if len(r.Lines) != 1 {
		t.Fatalf("expected one row, got %q", r.Lines)
	}
	if got := ansi.Strip(r.Lines[0]); got != quoteBar+" say loud" {
		t.Errorf("rich_text quote text %q", got)
	}
	if bold := sgrPrefix(t, boldStyle()); !strings.Contains(r.Lines[0], bold) {
		t.Errorf("rich_text quote lacks bold SGR %q:\n%q", bold, r.Lines[0])
	}
}

func TestCommonMark_QuoteConvertsInline(t *testing.T) {
	names := map[string]string{"U1": "alice"}
	cases := map[string]string{
		"> *loud* <@U1> &amp; <https://example.com|docs>": "> **loud** @alice & [docs](https://example.com)",
		"> &lt;@U1&gt;": "> <@U1>",
		"&gt; run `ls`": "> run `ls`",
	}
	for in, want := range cases {
		if got := SlackMrkdwnToCommonMark(in, names, nil); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func richTextLiteralMessage() MessageItem {
	return MessageItem{TS: "1.0", Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "ping <@U1> then "},
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "a < b", Style: &slack.RichTextSectionTextStyle{Code: true}},
		}},
	}}}}
}

func TestRichTextLiteralMentionRendersAsText(t *testing.T) {
	msg := richTextLiteralMessage()
	opts := RenderSlackMarkdownOpts{UserNames: map[string]string{"U1": "alice"}, Width: 60}
	got := ansi.Strip(RenderSlackMarkdownWith(MessageTextSource(msg), opts))
	if !strings.Contains(got, "ping <@U1> then a < b") || strings.Contains(got, "alice") {
		t.Errorf("literal rich_text characters reinterpreted: %q", got)
	}
	if plain := renderedFor(t, msg, 80); !strings.Contains(plain, "ping <@U1> then a < b") {
		t.Errorf("messages pane reinterpreted literal rich_text characters:\n%s", plain)
	}
}

func TestSlackMrkdwnToCommonMark_DecodesEntitiesInsideCode(t *testing.T) {
	in := "`a &lt; b`\n```\nx &amp;&amp; y &gt; z\n```"
	want := "`a < b`\n```\nx && y > z\n```"
	if got := SlackMrkdwnToCommonMark(in, nil, nil); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCodeBlock_UnknownFirstLineStaysCode(t *testing.T) {
	for _, code := range []string{"ERROR\nboom", "notalanguage\nx", "make\nmake test", "text\nhello", "c\nd"} {
		out := RenderSlackMarkdownWith("```\n"+code+"\n```", RenderSlackMarkdownOpts{Width: 40})
		if plain := ansi.Strip(out); !strings.Contains(plain, strings.Split(code, "\n")[0]) {
			t.Errorf("first line of %q eaten as a language: %q", code, plain)
		}
	}
}

func TestCodeBlock_HighlightReopensOnWrappedRows(t *testing.T) {
	withDarkTheme(t)
	code := "s := \"" + strings.Repeat("a", 40) + "\""
	out := RenderSlackMarkdownWith("```<go>\n"+code+"\n```", RenderSlackMarkdownOpts{Width: 20})
	var rows []string
	for _, r := range strings.Split(out, "\n") {
		if strings.Contains(r, "aaaa") {
			rows = append(rows, r)
		}
	}
	if len(rows) < 2 {
		t.Fatalf("expected the string to wrap, got %q", rows)
	}
	i := strings.IndexByte(rows[1], 'a')
	if i < 0 || !strings.Contains(rows[1][:i], fgANSIFor(styles.Accent)) {
		t.Errorf("wrapped string row does not re-open the string color: %q", rows[1])
	}
}

func TestSlackMrkdwnToCommonMark_CarriesFenceLanguage(t *testing.T) {
	cases := map[string]string{
		"```<go>\nx := 1\n```":         "```go\nx := 1\n```",
		"```<plain_text>\nx := 1\n```": "```plain_text\nx := 1\n```",
		"```\nERROR\nboom```":          "```\nERROR\nboom\n```",
		"```\nmake\nmake test```":      "```\nmake\nmake test\n```",
		"```&lt;go&gt;\nx```":          "```\n<go>\nx\n```",
	}
	for in, want := range cases {
		if got := SlackMrkdwnToCommonMark(in, nil, nil); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// The non-blank rows inside the frame, in order, with the frame and the
// right padding removed and the left indentation kept.
func codeRows(out string) []string {
	var code []string
	for _, r := range nonBlankRows(out) {
		if !strings.HasPrefix(r, "│") {
			continue
		}
		inner := strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(r, "│ "), "│"), " ")
		if inner != "" {
			code = append(code, inner)
		}
	}
	return code
}

func requireCodeRows(t *testing.T, name, out string, want ...string) {
	t.Helper()
	if got := codeRows(out); !slices.Equal(got, want) {
		t.Errorf("%s: code rows %q, want %q", name, got, want)
	}
}

func TestCodeBlock_FramedWithLanguageLabel(t *testing.T) {
	withDarkTheme(t)
	const width = 30
	out := RenderSlackMarkdownWith("```<go>\nreturn 1\n```", RenderSlackMarkdownOpts{Width: width})
	rows := nonBlankRows(out)
	if len(rows) != 6 {
		t.Fatalf("want top border, label, blank, code, blank, bottom border; got %q", rows)
	}
	for i, want := range []string{"╭", "│ Go", "│  ", "│ 1  return 1", "│  ", "╰"} {
		if !strings.HasPrefix(rows[i], want) {
			t.Errorf("row %d = %q, want prefix %q", i, rows[i], want)
		}
		if lipgloss.Width(rows[i]) != width {
			t.Errorf("row %d is %d wide, want the message width %d: %q", i, lipgloss.Width(rows[i]), width, rows[i])
		}
	}
	for _, i := range []int{2, 4} {
		if strings.TrimSpace(strings.Trim(rows[i], "│")) != "" {
			t.Errorf("padding row %d is not blank: %q", i, rows[i])
		}
	}
	if !strings.Contains(out, fgANSIFor(styles.TextMuted)+"Go") {
		t.Errorf("label is not muted: %q", out)
	}
	if !strings.Contains(out, fgANSIFor(styles.Primary)+"return") {
		t.Errorf("keyword not painted in the theme's Primary: %q", out)
	}
}

func TestPaintSpans_OneChangePerColorRun(t *testing.T) {
	withDarkTheme(t)
	got := paintSpans([]syntax.Span{{Text: "a", Color: styles.Primary}, {Text: "b", Color: styles.Primary}, {Text: "c", Color: styles.Accent}})
	if want := fgANSIFor(styles.Primary) + "ab" + fgANSIFor(styles.Accent) + "c"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCodeBlock_UntaggedIsPaddedWithoutLabel(t *testing.T) {
	out := RenderSlackMarkdownWith("```\nERROR\n```", RenderSlackMarkdownOpts{Width: 30})
	rows := nonBlankRows(out)
	if len(rows) != 5 || !strings.HasPrefix(rows[2], "│ ERROR") {
		t.Fatalf("want top border, blank, code, blank, bottom border; got %q", rows)
	}
}

// T text row, . blank row, B a run of box rows: a box collapses to one
// letter, every blank or text row keeps its own so the gaps are counted.
func rowPattern(out string) string {
	var p []byte
	for _, r := range strippedRows(out) {
		kind := byte('T')
		switch {
		case strings.TrimSpace(r) == "":
			kind = '.'
		case strings.HasPrefix(r, "╭"), strings.HasPrefix(r, "│"), strings.HasPrefix(r, "╰"):
			kind = 'B'
		}
		if len(p) == 0 || p[len(p)-1] != kind || kind != 'B' {
			p = append(p, kind)
		}
	}
	return string(p)
}

func TestCodeBlock_OneBlankRowFromNeighbours(t *testing.T) {
	cases := map[string]string{
		"text\n\n\n```\nx\n```\n\n\nmore": "T.B.T",
		"text\n```\nx\n```\nmore":         "T.B.T",
		"```\nx\n```\n\n```\ny\n```":      "B.B",
		"```\nx\n```\nafter":              "B.T",
		"before\n```\nx\n```":             "T.B",
	}
	for in, want := range cases {
		if got := rowPattern(RenderSlackMarkdownWith(in, RenderSlackMarkdownOpts{Width: 30})); got != want {
			t.Errorf("%q: rows %s, want %s", in, got, want)
		}
	}
}

func TestCodeBlock_TaggedRowsAreNumbered(t *testing.T) {
	withDarkTheme(t)
	code := "a := 1\nb := \"" + strings.Repeat("x", 40) + "\"\nc := 3"
	out := RenderSlackMarkdownWith("```<go>\n"+code+"\n```", RenderSlackMarkdownOpts{Width: 30})
	requireCodeRows(t, "numbered code", out, "Go", "1  a := 1", "2  b := \""+strings.Repeat("x", 17), "   "+strings.Repeat("x", 23), "   \"", "3  c := 3")
	if !strings.Contains(out, fgANSIFor(styles.TextMuted)+"1  ") {
		t.Errorf("gutter is not muted: %q", out)
	}
}

func TestCodeBlock_GutterWidensWithLineCount(t *testing.T) {
	code := strings.TrimSuffix(strings.Repeat("x\n", 10), "\n")
	rows := codeRows(RenderSlackMarkdownWith("```<go>\n"+code+"\n```", RenderSlackMarkdownOpts{Width: 30}))[1:]
	if len(rows) != 10 || rows[8] != " 9  x" || rows[9] != "10  x" {
		t.Errorf("gutter did not widen to two digits: %q", rows)
	}
}

func TestCodeBlock_SearchTermsSkipTheGutter(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("```<go>\nx := 2\ny := 0\n```", RenderSlackMarkdownOpts{Width: 30, SearchTerms: []string{"2"}})
	if n := strings.Count(out, hlStart+"2"+hlEnd); n != 1 {
		t.Errorf("term highlighted %d times, want once (code only, not the gutter): %q", n, out)
	}
}

func TestCodeBlock_TaggedUnknownLanguageRendersUntagged(t *testing.T) {
	out := RenderSlackMarkdownWith("```<plain_text>\nx := 1\n```", RenderSlackMarkdownOpts{Width: 30})
	rows := nonBlankRows(out)
	if len(rows) != 5 || !strings.HasPrefix(rows[2], "│ x := 1") || strings.Contains(out, "plain_text") {
		t.Errorf("want the untagged five-row box with the tag gone, got %q", rows)
	}
}

func TestCodeBlock_EmptyTaggedBlockKeepsItsLabel(t *testing.T) {
	out := RenderSlackMarkdownWith("```<go>\n\n```", RenderSlackMarkdownOpts{Width: 30})
	requireCodeRows(t, "empty tagged", out, "Go", "1")
}

func TestCodeBlock_PreviewIsBareCodeRows(t *testing.T) {
	out := RenderSlackMarkdownWith("```<go>\nx := 1\n```", RenderSlackMarkdownOpts{Preview: true})
	plain := ansi.Strip(out)
	if strings.ContainsAny(plain, "╭╰│") || strings.Contains(plain, "Go") || strings.Contains(plain, "1  ") {
		t.Errorf("preview carries frame, label or gutter: %q", plain)
	}
	if !strings.Contains(plain, "x := 1") {
		t.Errorf("preview lost the code: %q", plain)
	}
}

func TestCodeBlock_SearchTermSpansTokenColors(t *testing.T) {
	hlStart, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("```<go>\nfunc main() {}\n```", RenderSlackMarkdownOpts{Width: 40, SearchTerms: []string{"func main"}})
	if a, b := strings.Index(out, hlStart+"func"), strings.Index(out, "main"+hlEnd); a < 0 || b < a {
		t.Errorf("term across a keyword/name boundary not highlighted as one run: %q", out)
	}
}

func TestCodeBlock_HighlightFollowsThemeAcrossRenders(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	dark := RenderSlackMarkdownWith("```<go>\nfunc\n```", RenderSlackMarkdownOpts{Width: 30})
	styles.Apply("dracula", config.Theme{})
	dracula := RenderSlackMarkdownWith("```<go>\nfunc\n```", RenderSlackMarkdownOpts{Width: 30})
	if dark == dracula || !strings.Contains(dracula, fgANSIFor(styles.Primary)+"func") {
		t.Errorf("second render of the same block kept the old theme's colors: %q", dracula)
	}
}

func TestReopenSGRAcrossRows_ClosersRetireOpensAndAreNotCarried(t *testing.T) {
	const fg, bg, bold = "\x1b[38;2;1;2;3m", "\x1b[48;5;7m", "\x1b[1m"
	cases := []struct {
		name string
		rows []string
		want string
	}{
		{"kitty placeholder closes its fg", []string{"a " + fg + "\U0010EEEE\x1b[39m b", "next"}, "next"},
		{"bg closer", []string{bg + "x\x1b[49m", "next"}, "next"},
		{"bold off keeps the open fg", []string{bold + "b " + fg + "red\x1b[22m", "next"}, fg + "next"},
		{"reset clears everything", []string{bold + fg + "x\x1b[m", "next"}, "next"},
		{"open fg carries", []string{fg + "x", "next"}, fg + "next"},
	}
	for _, c := range cases {
		if got := reopenSGRAcrossRows(c.rows)[1]; got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCodeBlock_SearchHighlightKeepsTheSurfaceBehindTheRestOfTheRow(t *testing.T) {
	_, hlEnd := searchHighlightSGRForTest(t)
	out := RenderSlackMarkdownWith("```\nfunc deploy() {}\n```", RenderSlackMarkdownOpts{Width: 40, SearchTerms: []string{"deploy"}})
	rest := out[strings.Index(out, "deploy")+len("deploy"):]
	rest = rest[strings.Index(rest, hlEnd)+len(hlEnd):]
	var reapplied []string
	for m := sgrRe.FindStringIndex(rest); m != nil && m[0] == 0; m = sgrRe.FindStringIndex(rest) {
		reapplied = append(reapplied, rest[:m[1]])
		rest = rest[m[1]:]
	}
	if !slices.Contains(reapplied, bgANSIFor(styles.Surface)) {
		t.Errorf("box background not restored after the highlight's reset; re-applied %q", reapplied)
	}
}

// Below frame plus gutter there is nothing to wrap to; the box keeps its
// natural width rather than letting lipgloss shred it into one-cell rows.
func TestCodeBlock_TooNarrowForTheFrameKeepsRowsWhole(t *testing.T) {
	for _, width := range []int{1, 5, 7} {
		requireCodeRows(t, fmt.Sprint("width ", width), RenderSlackMarkdownWith("```<go>\nreturn 1\n```", RenderSlackMarkdownOpts{Width: width}), "Go", "1  return 1")
	}
}

func TestCodeBlock_TypedEntityTagStaysLiteral(t *testing.T) {
	out := RenderSlackMarkdownWith("```&lt;go&gt;\nx := 1\n```", RenderSlackMarkdownOpts{Width: 30})
	requireCodeRows(t, "escaped tag", out, "<go>", "x := 1")
}

// SlackMrkdwnToCommonMarkWithUserGroups (upstream) spells the held-block
// placeholder by hand; this pins the fork's helper to that spelling.
func TestCodeBlockMarker_MatchesTheCommonMarkPlaceholder(t *testing.T) {
	if got := codeBlockMarker(7); got != fmt.Sprintf("\x00CB%d\x00", 7) {
		t.Errorf("marker %q diverged from the CommonMark placeholder format", got)
	}
	if !isCodeBlockMarker(fmt.Sprintf("\x00CB%d\x00", 12)) {
		t.Error("CommonMark-shaped placeholder not recognised as a marker")
	}
}
