package blockkit

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/styles"
)

func richTextBlockOf(text string) RichTextBlock {
	return rtSection(&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: text})
}

func wrapWords(s string, limit int) string { return ansi.Wordwrap(s, limit, "") }

func plainLines(r RenderResult) []string {
	out := make([]string, len(r.Lines))
	for i, l := range r.Lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

var twoByTwo = TableBlock{Rows: [][]string{
	{"Step", "Adapter does"},
	{"Auth()", "proves it can reach the service"},
}}

func TestRenderTableBoxHeaderRuleAndRows(t *testing.T) {
	r := Render([]Block{twoByTwo}, makeCtx(), 80)
	lines := plainLines(r)
	if len(lines) != 5 {
		t.Fatalf("Lines = %q, want top border, header, rule, one body row, bottom border", lines)
	}
	top, header, rule, row, bottom := lines[0], lines[1], lines[2], lines[3], lines[4]
	if strings.Trim(top, "┌┬┐─") != "" || !strings.Contains(top, "┬") {
		t.Errorf("top = %q, want a ┌─┬─┐ border", top)
	}
	if !strings.Contains(header, "Step") || !strings.Contains(header, "Adapter does") || !strings.HasPrefix(header, "│") || !strings.HasSuffix(header, "│") {
		t.Errorf("header = %q, want both header cells inside │ borders", header)
	}
	if strings.Trim(rule, "├┼┤─") != "" || !strings.Contains(rule, "┼") {
		t.Errorf("rule = %q, want a ├─┼─┤ rule", rule)
	}
	if !strings.Contains(row, "Auth()") || !strings.Contains(row, "proves it can reach the service") {
		t.Errorf("row = %q, want both body cells", row)
	}
	if strings.Trim(bottom, "└┴┘─") != "" || !strings.Contains(bottom, "┴") {
		t.Errorf("bottom = %q, want a └─┴─┘ border", bottom)
	}
	want := columnsOf(header, "│")
	for name, got := range map[string][]int{
		"top":    columnsOf(top, "┌┬┐"),
		"rule":   columnsOf(rule, "├┼┤"),
		"row":    columnsOf(row, "│"),
		"bottom": columnsOf(bottom, "└┴┘"),
	} {
		if !slices.Equal(got, want) {
			t.Errorf("%s joints at %v, header bars at %v:\n%s", name, got, want, strings.Join(lines, "\n"))
		}
	}
}

// columnsOf lists the display columns of every rune of glyphs in line.
func columnsOf(line, glyphs string) []int {
	var cols []int
	col := 0
	for _, r := range line {
		if strings.ContainsRune(glyphs, r) {
			cols = append(cols, col)
		}
		col += lipgloss.Width(string(r))
	}
	return cols
}

func TestRenderTableRulesBetweenEveryRow(t *testing.T) {
	three := TableBlock{Rows: [][]string{{"Step", "Adapter does"}, {"Auth()", "proves it"}, {"Capture()", "writes it"}}}
	lines := plainLines(Render([]Block{three}, makeCtx(), 80))
	want := []string{"┌", "│", "├", "│", "├", "│", "└"}
	if len(lines) != len(want) {
		t.Fatalf("Lines = %q, want a rule between every row (%d lines)", lines, len(want))
	}
	for i, prefix := range want {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("Lines[%d] = %q, want it to start with %q", i, lines[i], prefix)
		}
	}
}

func TestRenderTableWrapsCellsToFitWidth(t *testing.T) {
	const width = 30
	ctx := makeCtx()
	ctx.WrapText = wrapWords
	r := Render([]Block{twoByTwo}, ctx, width)
	joined := strings.Join(plainLines(r), "\n")
	for _, l := range plainLines(r) {
		if lipgloss.Width(l) > width {
			t.Errorf("line %q is %d cols, want <= %d", l, lipgloss.Width(l), width)
		}
	}
	for _, w := range []string{"Step", "Adapter", "Auth()", "proves", "reach", "service"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q after wrapping:\n%s", w, joined)
		}
	}
	if len(r.Lines) <= 5 {
		t.Errorf("expected the long cell to wrap onto extra rows, got:\n%s", joined)
	}
	if !strings.Contains(joined, "│ Auth()") || !strings.Contains(joined, "│        ") {
		t.Errorf("wrapped row must keep its first column blank on continuation lines:\n%s", joined)
	}
}

func TestRenderTableCellsGoThroughRenderText(t *testing.T) {
	ctx := makeCtx()
	ctx.RenderText = func(s string, _ map[string]string) string { return strings.ToUpper(s) }
	r := Render([]Block{twoByTwo}, ctx, 80)
	if !strings.Contains(strings.Join(plainLines(r), "\n"), "PROVES IT CAN REACH THE SERVICE") {
		t.Errorf("cells bypassed RenderText:\n%s", strings.Join(plainLines(r), "\n"))
	}
}

func TestRenderDrawsRichTextInPlace(t *testing.T) {
	r := Render([]Block{richTextBlockOf("nested"), DividerBlock{}}, makeCtx(), 40)
	if lines := plainLines(r); len(lines) != 2 || lines[0] != "nested" {
		t.Errorf("Lines = %q, want the rich_text drawn before the divider", lines)
	}
}

func TestRenderMessageBlocksSkipsFirstRichTextWithContent(t *testing.T) {
	r := RenderMessageBlocks([]Block{DividerBlock{}, rtSection(), richTextBlockOf("first"), richTextBlockOf("second")}, makeCtx(), 40)
	if lines := plainLines(r); len(lines) != 2 || lines[1] != "second" {
		t.Errorf("Lines = %q, want the empty rich_text ignored, \"first\" skipped as the body, \"second\" drawn", lines)
	}
}

func TestRenderTableNeverExceedsWidth(t *testing.T) {
	long := TableBlock{Rows: [][]string{{"alpha beta gamma", "delta epsilon zeta", "eta theta iota"}, {"one two three four", "five six", "seven eight nine ten"}}}
	const width = 40
	ctx := makeCtx()
	ctx.WrapText = wrapWords
	for i, line := range Render([]Block{long}, ctx, width).Lines {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("line %d is %d cols, want <= %d: %q", i, w, width, ansi.Strip(line))
		}
	}
}

// A table the pane cannot give tableMinCellW cells per column becomes a
// one-line badge: wrapping words into fragments would make every row many
// lines tall.
func TestRenderTableTooWideBecomesBadge(t *testing.T) {
	wide := TableBlock{Rows: [][]string{{"a", "b", "c", "d", "e"}, {"alpha beta", "gamma delta", "epsilon", "zeta eta", "theta iota"}}}
	const width = 20
	ctx := makeCtx()
	ctx.WrapText = wrapWords
	lines := plainLines(Render([]Block{wide}, ctx, width))
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "[table: 5 columns") || lipgloss.Width(lines[0]) > width {
		t.Errorf("Lines = %q, want a single badge naming the 5-column table within %d cols", lines, width)
	}
}

func TestRenderMessageBlocksSkipsBodyRichTextOnly(t *testing.T) {
	r := RenderMessageBlocks([]Block{richTextBlockOf("first"), DividerBlock{}, richTextBlockOf("second")}, makeCtx(), 40)
	lines := plainLines(r)
	if len(lines) != 2 {
		t.Fatalf("Lines = %q, want first-block omitted, then divider and second-block text", lines)
	}
	if strings.Contains(strings.Join(lines, "\n"), "first") {
		t.Errorf("first rich_text block is the host-rendered body and must not repeat: %q", lines)
	}
	if lines[1] != "second" {
		t.Errorf("Lines[1] = %q, want the second rich_text block rendered after the divider", lines[1])
	}
}

func TestRenderMessageBlocksTableBetweenParagraphs(t *testing.T) {
	p := loadFixture(t, "table_between_paragraphs.json")
	blocks := Parse(p.Blocks)
	for _, w := range []int{60, 100, 140} {
		ctx := makeCtx()
		ctx.WrapText = wrapWords
		r := RenderMessageBlocks(blocks, ctx, w)
		plain := strings.Join(plainLines(r), "\n")
		if strings.Contains(plain, "unsupported block") {
			t.Errorf("width=%d table rendered as unsupported:\n%s", w, plain)
		}
		if strings.Contains(plain, "bare verbs") {
			t.Errorf("width=%d body paragraph repeated below the body:\n%s", w, plain)
		}
		for _, want := range []string{"Step", "Adapter does", "Auth()", "Capture()", "Candidates for the 4th step", "Manifest()", "DescribeCapture()", "Configure"} {
			if !strings.Contains(plain, want) {
				t.Errorf("width=%d missing %q:\n%s", w, want, plain)
			}
		}
		if strings.Index(plain, "Capture()") > strings.Index(plain, "Candidates") {
			t.Errorf("width=%d table must precede the second paragraph:\n%s", w, plain)
		}
		for _, l := range plainLines(r) {
			if lipgloss.Width(l) > w {
				t.Errorf("width=%d line %q overflows", w, l)
			}
		}
	}
}

// Slack sends raw_text / raw_number cells, and null for empty ones, for
// tables not composed as rich text.
func TestRenderMessageBlocksRawTextTableCells(t *testing.T) {
	p := loadFixture(t, "table_raw_text_cells.json")
	r := RenderMessageBlocks(Parse(p.Blocks), makeCtx(), 120)
	plain := strings.Join(plainLines(r), "\n")
	for _, want := range []string{"docs", "today", "after", "snapshotter README", "255 lines; hand-kept roster", "the one README", "7 per-service hydration plans", "~1,980 lines"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q:\n%s", want, plain)
		}
	}
}

// Block text is concatenated after the host's styled body, so it must
// carry the theme foreground itself or fall back to the terminal default.
func TestRichTextAndTableCellsCarryMessageForeground(t *testing.T) {
	probe := styles.MessageText.Render("x")
	fg := probe[len("\x1b["):strings.IndexByte(probe, 'm')]
	r := Render([]Block{richTextBlockOf("paragraph"), twoByTwo}, makeCtx(), 80)
	for i, line := range r.Lines {
		if ansi.Strip(line) == "" || strings.Trim(ansi.Strip(line), "┌┬┐├┼┤└┴┘─") == "" {
			continue
		}
		if !strings.Contains(line, fg) {
			t.Errorf("line %d lacks the MessageText foreground %q:\n%q", i, fg, line)
		}
	}
}

func TestRenderUnsupportedWearsWarningBadge(t *testing.T) {
	probe := lipgloss.NewStyle().Background(styles.Warning).Render("x")
	warningBg := probe[len("\x1b["):strings.IndexByte(probe, 'm')]
	if got := renderUnsupported("video", 80); !strings.Contains(got, warningBg) {
		t.Errorf("unsupported marker %q lacks the Warning background %q", got, warningBg)
	}
}

// Text is rendered for the exact width it is then wrapped to, including
// where a block narrows its text column (an accessory beside a section,
// the stripe of a legacy attachment).
func TestRenderTextForWidthMatchesWrapWidth(t *testing.T) {
	var renderW, wrapW []int
	ctx := Context{
		RenderTextForWidth: func(s string, _ map[string]string, width int) string {
			renderW = append(renderW, width)
			return s
		},
		WrapText: func(s string, width int) string {
			wrapW = append(wrapW, width)
			return s
		},
	}
	Render([]Block{
		SectionBlock{Text: "> quoted", Accessory: LabelAccessory{Kind: "button", Label: "Deploy"}},
		SectionBlock{Text: "plain", Fields: []string{"a", "b"}},
	}, ctx, 80)
	RenderLegacy([]LegacyAttachment{{Pretext: "pre", Text: "> quoted"}}, ctx, 80)
	if len(renderW) < 5 || !slices.Equal(renderW, wrapW) {
		t.Errorf("render widths %v, wrap widths %v: every text must be rendered for the width it is wrapped to", renderW, wrapW)
	}
	if slices.Equal(renderW, []int{80, 80, 80, 80, 80, 80}) {
		t.Errorf("fixture never narrowed the text column: %v", renderW)
	}
}
