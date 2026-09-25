package messages

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

func TestCodeBlocks(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []CodeBlock
	}{
		{"no block", "plain text", nil},
		{"inline code is not a block", "run `go test` and `go vet`", nil},
		{"one block", "before\n```\nfmt.Println(1)\n```\nafter", []CodeBlock{{Code: "fmt.Println(1)"}}},
		{"many blocks keep message order", "```one```\nbetween `x`\n```\ntwo\n```", []CodeBlock{{Code: "one"}, {Code: "two"}}},
		{"language tag stripped", "```<go>\nreturn 1\n```", []CodeBlock{{Language: "go", Code: "return 1"}}},
		{"entities decoded", "```if a &lt; b &amp;&amp; c &gt; d {}```", []CodeBlock{{Code: "if a < b && c > d {}"}}},
		{"one fence newline trimmed per side", "```\n\n  indented\n\n```", []CodeBlock{{Code: "\n  indented\n"}}},
		{"crlf fence newlines trimmed", "```\r\ncode\r\n```", []CodeBlock{{Code: "code"}}},
		{"tabs preserved", "```\n\tif x {\n\t\treturn\n\t}\n```", []CodeBlock{{Code: "\tif x {\n\t\treturn\n\t}"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CodeBlocks(MessageItem{Text: tc.text}); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("CodeBlocks(%q) = %#v, want %#v", tc.text, got, tc.want)
			}
		})
	}
}

func TestCodeBlocks_ReadsTheRichTextBody(t *testing.T) {
	msg := MessageItem{Text: "fallback without a fence", Blocks: []blockkit.Block{
		blockkit.RichTextBlock{Elements: []slack.RichTextElement{&slack.RichTextPreformatted{
			Type:     slack.RTEPreformatted,
			Language: "go",
			Elements: []slack.RichTextSectionElement{&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "a < b"}},
		}}},
	}}
	want := []CodeBlock{{Language: "go", Code: "a < b"}}
	if got := CodeBlocks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("CodeBlocks = %#v, want %#v (source %q)", got, want, MessageTextSource(msg))
	}
}

// Block N of CodeBlocks is block N on screen: each block's code, tabs
// expanded the way the renderer paints them, is what the renderer drew.
func TestCodeBlocks_MatchWhatTheRendererDraws(t *testing.T) {
	text := "intro\n```\n\n\tfirst &lt;1&gt;\n```\nmiddle ```second``` tail\n```<go>\n\nreturn 3\n\n```"
	blocks := CodeBlocks(MessageItem{Text: text})
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3: %#v", len(blocks), blocks)
	}
	var drawn []string
	for _, m := range codeBlockRe.FindAllString(text, -1) {
		out := ansi.Strip(RenderSlackMarkdownWith(m, RenderSlackMarkdownOpts{Preview: true}))
		var rows []string
		for _, row := range strings.Split(strings.Trim(out, "\n"), "\n") {
			rows = append(rows, strings.TrimRight(strings.TrimPrefix(row, " "), " "))
		}
		drawn = append(drawn, strings.Join(rows, "\n"))
	}
	for i, b := range blocks {
		if want := expandTabs(b.Code); drawn[i] != want {
			t.Errorf("block %d: renderer drew %q, CodeBlocks has %q", i, drawn[i], want)
		}
	}
}

// renderBodyWithCopyLabels is renderBody with the panes' opt-in: the
// wrapped rows, stripped, and the labels the renderer reported for them.
func renderBodyWithCopyLabels(src string, width int) ([]string, []CodeBlockCopyLabel) {
	var labels []CodeBlockCopyLabel
	out := RenderSlackMarkdownWith(src, RenderSlackMarkdownOpts{Width: width, CodeBlockCopyLabels: &labels})
	return strings.Split(ansi.Strip(WordWrap(out, width)), "\n"), labels
}

func TestCodeBlockCopyLabel_SitsOnTheTopBorder(t *testing.T) {
	const width = 40
	rows, labels := renderBodyWithCopyLabels("```\nx := 1\n```", width)
	want := "╭" + strings.Repeat("─", width-9) + " copy ─╮"
	if rows[0] != want {
		t.Errorf("top border = %q, want %q", rows[0], want)
	}
	if len(labels) != 1 || labels[0].Block != 0 || labels[0].Row != 0 {
		t.Fatalf("labels = %#v, want one for block 0 on row 0", labels)
	}
	if got := ansi.Cut(rows[0], labels[0].ColStart, labels[0].ColEnd); got != " copy " {
		t.Errorf("reported columns cover %q, want the label", got)
	}
}

// The reported row is a row of the wrapped body, whatever prose wraps
// above the block and however many blocks came before it.
func TestCodeBlockCopyLabel_ReportsRowsOfTheWrappedBody(t *testing.T) {
	prose := strings.Repeat("word ", 30)
	rows, labels := renderBodyWithCopyLabels(prose+"\n```\nfirst\n```\n• "+prose+"\n> "+prose+"\n```<go>\nsecond()\n```\ntail", 40)
	if len(labels) != 2 {
		t.Fatalf("labels = %#v, want 2", labels)
	}
	for i, l := range labels {
		if l.Block != i {
			t.Errorf("label %d is for block %d", i, l.Block)
		}
		if got := ansi.Cut(rows[l.Row], l.ColStart, l.ColEnd); got != " copy " || !strings.HasPrefix(rows[l.Row], "╭") {
			t.Errorf("label %d points at %q in row %d %q", i, got, l.Row, rows[l.Row])
		}
	}
}

func TestCodeBlockCopyLabel_OnlyWhenTheCallerCollectsThem(t *testing.T) {
	for name, opts := range map[string]RenderSlackMarkdownOpts{
		"no collector": {Width: 40},
		"preview":      {Width: 40, Preview: true, CodeBlockCopyLabels: new([]CodeBlockCopyLabel)},
	} {
		out := ansi.Strip(RenderSlackMarkdownWith("```\nx := 1\n```", opts))
		if strings.Contains(out, "copy") {
			t.Errorf("%s: rendered a copy label:\n%s", name, out)
		}
		if opts.CodeBlockCopyLabels != nil && len(*opts.CodeBlockCopyLabels) != 0 {
			t.Errorf("%s: reported %#v", name, *opts.CodeBlockCopyLabels)
		}
	}
}

// A block too narrow for the label goes without, and the next block
// keeps its own index.
func TestCodeBlockCopyLabel_OmittedWhenTheBlockIsTooNarrow(t *testing.T) {
	rows, labels := renderBodyWithCopyLabels("```x```\n```wide enough for it```", 0)
	if rows[0] != "╭───╮" {
		t.Errorf("narrow block's top border = %q, want it bare", rows[0])
	}
	if len(labels) != 1 || labels[0].Block != 1 {
		t.Fatalf("labels = %#v, want only block 1", labels)
	}
	if got := ansi.Cut(rows[labels[0].Row], labels[0].ColStart, labels[0].ColEnd); got != " copy " {
		t.Errorf("block 1's label points at %q in %q", got, rows[labels[0].Row])
	}
}

// copyLabelCells finds the copy labels a pane drew: the pane-local
// (y, x) of each label's first cell, top to bottom.
func copyLabelCells(view string) [][2]int {
	var cells [][2]int
	for y, row := range strings.Split(ansi.Strip(view), "\n") {
		if before, _, found := strings.Cut(row, copyLabelText+"─╮"); found {
			cells = append(cells, [2]int{y, ansi.StringWidth(before)})
		}
	}
	return cells
}

// The labels are found on the drawn pane, not read back from the model,
// so a hit proves the region sits under the text: scrolled, beside an
// avatar, under a broadcast row.
func TestCodeBlockAt_ALabelGivesItsOwnBlocksCode(t *testing.T) {
	m := New([]MessageItem{
		{TS: "1.0", UserName: "alice", Text: "```\nscrolled away\n```\n" + strings.Repeat("filler\n", 40)},
		{TS: "2.0", UserName: "bob", Subtype: "thread_broadcast", Text: "two blocks\n```\nfirst\n```\nand\n```\n\tsecond &lt;2&gt;\n```"},
	}, "general")
	m.SetAvatarFunc(func(string) string { return "▀▀▀▀\n▄▄▄▄" })
	cells := copyLabelCells(m.View(24, 80))
	if len(cells) != 2 {
		t.Fatalf("found %d copy labels on screen, want 2", len(cells))
	}
	for i, want := range []string{"first", "\tsecond <2>"} {
		y, x := cells[i][0], cells[i][1]
		for _, col := range []int{x, x + len(copyLabelText) - 1} {
			if got, ok := m.CodeBlockAt(y, col); !ok || got != want {
				t.Errorf("label %d at (%d,%d): got %q ok=%v, want %q", i, y, col, got, ok, want)
			}
		}
		for _, miss := range [][2]int{{y, x - 1}, {y, x + len(copyLabelText)}, {y + 1, x}, {y - 1, x}} {
			if got, ok := m.CodeBlockAt(miss[0], miss[1]); ok {
				t.Errorf("(%d,%d) is off label %d but gave %q", miss[0], miss[1], i, got)
			}
		}
	}
}
