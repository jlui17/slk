package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
)

func TestBlockquote_BlockKitTextWrapsInsideBar(t *testing.T) {
	ctx := New().blockkitContext(messages.MessageItem{TS: "1.0"}, nil, nil)
	out := ctx.RenderTextForWidth("&gt; "+strings.Repeat("word ", 20), nil, 30)
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected the quote to wrap onto several rows, got %d: %q", len(lines), lines)
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, "┃") {
			t.Errorf("row %d lacks the quote bar: %q", i, l)
		}
	}
}
