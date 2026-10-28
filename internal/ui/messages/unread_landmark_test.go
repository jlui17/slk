package messages

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// The "── new ──" landmark: the channel pane draws it as a decoration row
// between the last read message and the first unread one. Nothing else
// asserts the rendered row (App tests check only the boundary state).
func TestUnreadLandmark_RendersBetweenReadAndUnread(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })

	items := []MessageItem{
		{TS: "1700000001.000000", UserName: "ann", Text: "msg-a", Timestamp: "9:00 AM"},
		{TS: "1700000002.000000", UserName: "bob", Text: "msg-b", Timestamp: "9:01 AM"},
		{TS: "1700000003.000000", UserName: "cat", Text: "msg-c", Timestamp: "9:02 AM"},
		{TS: "1700000004.000000", UserName: "dan", Text: "msg-d", Timestamp: "9:03 AM"},
	}
	m := New(items, "general")
	m.SetLastReadTS(items[1].TS)

	rows := p3MsgRows(m.View(40, 80))

	var landmarks []int
	for i, r := range rows {
		if strings.Contains(r, "── new ──") {
			landmarks = append(landmarks, i)
		}
	}
	if len(landmarks) != 1 {
		t.Fatalf("want exactly 1 landmark row, found %d:\n%s", len(landmarks), strings.Join(rows, "\n"))
	}
	read := p3MsgRowWith(t, rows, "msg-b")
	unread := p3MsgRowWith(t, rows, "msg-c")
	if !(read < landmarks[0] && landmarks[0] < unread) {
		t.Errorf("landmark at row %d, want after the last read message (row %d) and before the first unread (row %d):\n%s",
			landmarks[0], read, unread, strings.Join(rows, "\n"))
	}
}

func TestUnreadLandmark_AbsentWithoutBoundary(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })

	items := []MessageItem{
		{TS: "1700000001.000000", UserName: "ann", Text: "msg-a", Timestamp: "9:00 AM"},
		{TS: "1700000002.000000", UserName: "bob", Text: "msg-b", Timestamp: "9:01 AM"},
	}
	m := New(items, "general")
	if view := p3MsgRows(m.View(40, 80)); strings.Contains(strings.Join(view, "\n"), "── new ──") {
		t.Errorf("a pane with no boundary drew a landmark:\n%s", strings.Join(view, "\n"))
	}
}
