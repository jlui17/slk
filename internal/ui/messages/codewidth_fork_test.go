package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A code line longer than the text width widens its block, up to what
// the pane leaves, rather than wrapping mid-word; prose beside it still
// wraps at the text width, and the copy label follows the block's end.
func TestCodeBlockGrowsPastTheTextWidth(t *testing.T) {
	long := "gcp-colony-sandbox " + strings.Repeat("x", 100) // 119 cells
	m := New([]MessageItem{{TS: "1.0", UserName: "alice", Text: strings.Repeat("word ", 30) + "\n```\nshort\n" + long + "\n```"}}, "general")
	rows := strings.Split(ansi.Strip(m.View(40, 200)), "\n")
	codeRow := rowWith(t, rows, "gcp-colony-sandbox")
	if !strings.Contains(rows[codeRow], long) {
		t.Errorf("the long code line wrapped: %q", rows[codeRow])
	}
	for _, r := range rows {
		if strings.Contains(r, "word") && ansi.StringWidth(strings.TrimRight(r, " "))-TextCol > TextWidth(200) {
			t.Errorf("prose ran past the text width: %q", r)
		}
	}
	y, x := -1, -1
	for i, r := range rows {
		if before, _, ok := strings.Cut(r, copyLabelText); ok {
			y, x = i, ansi.StringWidth(before)
		}
	}
	block := len(long) + 2 // the line and the block's padding
	if want := TextCol + block - len(copyLabelText); x != want {
		t.Errorf("copy label at column %d, want %d (the end of a %d-cell block)", x, want, block)
	}
	if got, ok := m.CodeBlockAt(y, x+1); !ok || !strings.HasSuffix(got, long) {
		t.Errorf("CodeBlockAt(label) = %q, %v", got, ok)
	}
}

// Code wraps only where even the pane is too narrow for it, and a block
// of short lines stays the text width.
func TestCodeBlockWidthBounds(t *testing.T) {
	opts := RenderSlackMarkdownOpts{Width: 80, CodeWidth: 190}
	if w := codeBlockWidth("short", opts); w != 80 {
		t.Errorf("short code: block %d, want the text width 80", w)
	}
	if w := codeBlockWidth(strings.Repeat("x", 100), opts); w != 102 {
		t.Errorf("100-cell line: block %d, want 102", w)
	}
	if w := codeBlockWidth(strings.Repeat("x", 300), opts); w != 190 {
		t.Errorf("300-cell line: block %d, want the pane's 190", w)
	}
	out := RenderSlackMarkdownWith("```\n"+strings.Repeat("x", 300)+"\n```", opts)
	if rows := codeRows(out); len(rows) != 2 || len(rows[0]) != 188 {
		t.Errorf("past the pane the code wraps at the block: %d rows, %q", len(rows), rows)
	}
}
