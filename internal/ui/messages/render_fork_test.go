package messages

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
// barred rows alone.
func TestBlockquote_WrapsInsideBar(t *testing.T) {
	const width = 30
	out := RenderSlackMarkdownWith("> "+strings.Repeat("word ", 20), RenderSlackMarkdownOpts{Width: width})
	for name, pass := range map[string]string{"rendered": out, "rendered+WordWrap": WordWrap(out, width)} {
		lines := strippedRows(pass)
		requireBarredRows(t, name, lines, 3)
		for i, l := range lines {
			if w := lipgloss.Width(l); w > width {
				t.Errorf("%s: row %d is %d wide, limit %d: %q", name, i, w, width, l)
			}
		}
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
