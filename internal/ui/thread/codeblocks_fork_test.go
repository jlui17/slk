package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
)

// copyLabelCells finds the copy labels the pane drew: the pane-local
// (y, x) of each label's first cell, top to bottom.
func copyLabelCells(view string) [][2]int {
	var cells [][2]int
	for y, row := range strings.Split(ansi.Strip(view), "\n") {
		if before, _, found := strings.Cut(row, " copy ─╮"); found {
			cells = append(cells, [2]int{y, ansi.StringWidth(before)})
		}
	}
	return cells
}

// The labels are found on the drawn pane, not read back from the model,
// so a hit proves the region sits under the text.
func assertLabelsGive(t *testing.T, m *Model, view string, want []string) {
	t.Helper()
	cells := copyLabelCells(view)
	if len(cells) != len(want) {
		t.Fatalf("found %d copy labels on screen, want %d", len(cells), len(want))
	}
	for i, code := range want {
		y, x := cells[i][0], cells[i][1]
		for _, col := range []int{x, x + 5} {
			if got, ok := m.CodeBlockAt(y, col); !ok || got != code {
				t.Errorf("label %d at (%d,%d): got %q ok=%v, want %q", i, y, col, got, ok, code)
			}
		}
		for _, miss := range [][2]int{{y, x - 1}, {y, x + 6}, {y + 1, x}, {y - 1, x}} {
			if got, ok := m.CodeBlockAt(miss[0], miss[1]); ok {
				t.Errorf("(%d,%d) is off label %d but gave %q", miss[0], miss[1], i, got)
			}
		}
	}
}

func TestCodeBlockAt_ParentAndReplyLabelsGiveTheirOwnCode(t *testing.T) {
	m := New()
	parent := messages.MessageItem{TS: "1.0", UserName: "alice", Text: "```\nfrom the parent\n```"}
	replies := []messages.MessageItem{
		{TS: "1.001", UserName: "bob", Text: "no blocks here"},
		{TS: "1.002", UserName: "bob", Text: "two blocks\n```\nfirst\n```\nand\n```\n\tsecond &lt;2&gt;\n```"},
	}
	m.SetThread(parent, replies, "C1", "1.0")
	assertLabelsGive(t, m, m.View(40, 80), []string{"from the parent", "first", "\tsecond <2>"})
}

func TestCodeBlockAt_ScrolledPane(t *testing.T) {
	m := New()
	parent := messages.MessageItem{TS: "1.0", UserName: "alice", Text: "```\nscrolled away\n```\n" + strings.Repeat("filler\n", 40)}
	replies := []messages.MessageItem{
		{TS: "1.001", UserName: "bob", Text: "```\nin view\n```"},
	}
	m.SetThread(parent, replies, "C1", "1.0")
	m.MoveDown()
	assertLabelsGive(t, m, m.View(20, 80), []string{"in view"})
}

func TestCodeBlockAt_ParentWithoutReplies(t *testing.T) {
	m := New()
	m.SetThread(messages.MessageItem{TS: "1.0", UserName: "alice", Text: "```\nalone\n```"}, nil, "C1", "1.0")
	assertLabelsGive(t, m, m.View(20, 80), []string{"alone"})
}
