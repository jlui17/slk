package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
)

// The reply box is as wide as the replies' text plus its border and
// padding, with its text in their column; a narrow pane gives it all.
func TestThreadReplyBox(t *testing.T) {
	if w, in := threadReplyBox(200); w != messages.TextWidth(200)+3 || in != messages.TextCol-2 {
		t.Errorf("threadReplyBox(200) = %d, %d", w, in)
	}
	if w, in := threadReplyBox(20); w != 20 || in != 0 {
		t.Errorf("threadReplyBox(20) = %d, %d; want the full pane", w, in)
	}
	rows := strings.Split(indentRows("ab\ncd", 3, 8), "\n")
	for _, r := range rows {
		if ansi.StringWidth(r) != 8 || !strings.HasPrefix(ansi.Strip(r), "   ") {
			t.Errorf("indented row %q", ansi.Strip(r))
		}
	}
}
