package messages

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
)

func TestRenderMessagePlainTableBetweenParagraphs(t *testing.T) {
	msg := MessageItem{
		TS:        "1700000000.000000",
		UserName:  "claude",
		UserID:    "U-BOT",
		Text:      "first paragraph  | Step | Adapter | second paragraph", // lossy
		Timestamp: "3:15 PM",
		Blocks: []blockkit.Block{
			blockkittest.Paragraph("first paragraph"),
			blockkit.TableBlock{Rows: [][]string{{"Step", "Adapter"}, {"Auth()", "proves it"}}},
			blockkittest.Paragraph("second paragraph"),
		},
	}
	plain := renderedFor(t, msg, 100)
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

// The selected variant is built by swapping the theme background for the
// tint, then reasserting the tint after every reset, so a block renderer
// may leave runs bare (table cells after a gutter, action-row gaps) and
// they still take the tint instead of the terminal default. The normal
// variant gets the same treatment from the view-level
// ReapplyBgAfterResets pass, outside this cache.
func TestSelectedVariantLeavesNoRunWithoutBackground(t *testing.T) {
	msg := MessageItem{
		TS:        "1700000000.000000",
		UserName:  "claude",
		Timestamp: "3:15 PM",
		Blocks: []blockkit.Block{
			blockkit.TableBlock{Rows: [][]string{{"Gap today", "Change"}, {"Capture never", "`dump.sh` records"}}},
			blockkit.ActionsBlock{Elements: []blockkit.ActionElement{{Kind: "button", Label: "Approve"}, {Kind: "button", Label: "Reject"}}},
		},
	}
	m := New([]MessageItem{msg}, "general")
	m.buildCache(60)
	for _, e := range m.cache {
		if e.msgIdx != 0 {
			continue
		}
		for variant, lines := range map[string][]string{"selected": e.linesSelected} {
			for i, line := range lines {
				if bare := blockkittest.RunesWithoutBackground(line); bare != "" {
					t.Errorf("%s line %d prints %q with no background set:\n%q", variant, i, bare, line)
				}
			}
		}
	}
}
