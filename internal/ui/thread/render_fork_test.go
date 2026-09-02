package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
)

func TestRenderThreadMessageTableBetweenParagraphs(t *testing.T) {
	m := New()
	msg := messages.MessageItem{
		TS:        "1700000003.000000",
		UserName:  "claude",
		Timestamp: "3:15 PM",
		Text:      "first paragraph  | Step | Adapter | second paragraph", // lossy
		Blocks: []blockkit.Block{
			blockkittest.Paragraph("first paragraph"),
			blockkit.TableBlock{Rows: [][]string{{"Step", "Adapter"}, {"Auth()", "proves it"}}},
			blockkittest.Paragraph("second paragraph"),
		},
	}
	got, _, _ := m.renderThreadMessage(msg, 100, nil, nil, false)
	plain := ansi.Strip(got)
	for _, want := range []string{"first paragraph", "Step", "Auth()", "second paragraph"} {
		if n := strings.Count(plain, want); n != 1 {
			t.Errorf("%q appears %d times, want exactly once:\n%s", want, n, plain)
		}
	}
	first, table, second := strings.Index(plain, "first paragraph"), strings.Index(plain, "Auth()"), strings.Index(plain, "second paragraph")
	if !(first < table && table < second) {
		t.Errorf("blocks out of order (first=%d table=%d second=%d):\n%s", first, table, second, plain)
	}
}

// Mirrors the messages pane's selected-variant contract: the tint is
// reasserted after every reset, so bare runs from block renderers never
// show the terminal default on the selected reply.
func TestThreadSelectedVariantLeavesNoRunWithoutBackground(t *testing.T) {
	m := New()
	parent := messages.MessageItem{TS: "1.0", UserID: "alice", UserName: "alice", Text: "p"}
	reply := messages.MessageItem{
		TS:        "1.001",
		UserName:  "claude",
		Timestamp: "3:15 PM",
		Blocks: []blockkit.Block{
			blockkit.TableBlock{Rows: [][]string{{"Gap today", "Change"}, {"Capture never", "`dump.sh` records"}}},
			blockkit.ActionsBlock{Elements: []blockkit.ActionElement{{Kind: "button", Label: "Approve"}, {Kind: "button", Label: "Reject"}}},
		},
	}
	m.SetThread(parent, []messages.MessageItem{reply}, "C1", "1.0")
	_ = m.View(40, 60)
	if len(m.cache) == 0 {
		t.Fatal("View() did not populate the render cache")
	}
	for _, e := range m.cache {
		for variant, lines := range map[string][]string{"selected": e.linesSelected} {
			for i, line := range lines {
				if bare := blockkittest.RunesWithoutBackground(line); bare != "" {
					t.Errorf("%s line %d prints %q with no background set:\n%q", variant, i, bare, line)
				}
			}
		}
	}
}
