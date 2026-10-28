package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrapRows_LongTimestampOnlyOnSelected(t *testing.T) {
	pinLongTSClock(t)
	items := longTSItems()
	m := New(items, "general")
	const width = 40
	r := RenderMessage(items[0], width, m.renderContext(), RenderOptions{})
	w := WrapRows(items[0], r, width, NewWrapStyles(true))

	if len(w.Lines) != len(w.Selected) || len(w.Lines) != len(w.Plain) {
		t.Fatalf("variants differ in height: lines=%d selected=%d plain=%d", len(w.Lines), len(w.Selected), len(w.Plain))
	}
	if w.ShortSrc != r.Content {
		t.Errorf("ShortSrc should be the short-timestamp content when the selected header changes")
	}
	if got := ansi.Strip(strings.Join(w.Lines, "\n")); strings.Contains(got, "Tue Sep 29") {
		t.Errorf("unselected rows carry the long timestamp:\n%s", got)
	}
	if got := ansi.Strip(strings.Join(w.Selected, "\n")); !strings.Contains(got, "Tue Sep 29, 3:40 PM") {
		t.Errorf("selected rows lack the long timestamp:\n%s", got)
	}
	for i, p := range w.Plain {
		if strings.Contains(p.Text, "▌") {
			t.Errorf("plain row %d carries the border column: %q", i, p.Text)
		}
	}
	short := NewWrapStyles(true).SelectedVariant(w.ShortSrc, width)
	if len(short) != len(w.Selected) {
		t.Errorf("short variant height %d != selected %d", len(short), len(w.Selected))
	}
}

func TestWrapRows_NoShortSrcWithoutLongTimestamp(t *testing.T) {
	pinLongTSClock(t)
	items := []MessageItem{{TS: "", UserID: "U1", UserName: "priya", Text: "hi", Timestamp: "3:40 PM"}}
	m := New(items, "general")
	r := RenderMessage(items[0], 40, m.renderContext(), RenderOptions{})
	if w := WrapRows(items[0], r, 40, NewWrapStyles(false)); w.ShortSrc != "" {
		t.Errorf("ShortSrc = %q, want empty when the selected header is unchanged", w.ShortSrc)
	}
}
