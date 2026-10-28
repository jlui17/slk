package thread

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

func landmarkThread(t *testing.T) (*Model, []messages.MessageItem) {
	t.Helper()
	parent := messages.MessageItem{TS: "1700000000.000000", UserName: "pat", Text: "parent-msg", Timestamp: "9:00 AM"}
	replies := []messages.MessageItem{
		{TS: "1700000001.000000", UserName: "ann", Text: "rep-a", Timestamp: "9:01 AM"},
		{TS: "1700000002.000000", UserName: "bob", Text: "rep-b", Timestamp: "9:02 AM"},
		{TS: "1700000003.000000", UserName: "cat", Text: "rep-c", Timestamp: "9:03 AM"},
		{TS: "1700000004.000000", UserName: "dan", Text: "rep-d", Timestamp: "9:04 AM"},
	}
	m := p3Thread(t, 0) // applies the dark theme and restores it on cleanup
	m.SetThread(parent, replies, "C1", parent.TS)
	return m, replies
}

// The thread pane draws the "── new ──" landmark between the last read
// reply and the first unread one; nothing else asserts the rendered row.
func TestUnreadLandmark_RendersBetweenReadAndUnreadReply(t *testing.T) {
	m, replies := landmarkThread(t)
	m.SetLastReadTS(replies[1].TS)

	rows := p3Rows(m.View(40, 80))

	var landmarks []int
	for i, r := range rows {
		if strings.Contains(r, "── new ──") {
			landmarks = append(landmarks, i)
		}
	}
	if len(landmarks) != 1 {
		t.Fatalf("want exactly 1 landmark row, found %d:\n%s", len(landmarks), strings.Join(rows, "\n"))
	}
	read := p3RowWith(t, rows, "rep-b")
	unread := p3RowWith(t, rows, "rep-c")
	if !(read < landmarks[0] && landmarks[0] < unread) {
		t.Errorf("landmark at row %d, want after the last read reply (row %d) and before the first unread (row %d):\n%s",
			landmarks[0], read, unread, strings.Join(rows, "\n"))
	}
}

// 8b: the thread clears its boundary when it opens a different thread,
// and keeps it when the same thread is set again (a refresh).
func TestSetThread_ClearsBoundaryOnlyWhenTheThreadChanges(t *testing.T) {
	m, replies := landmarkThread(t)
	parent := messages.MessageItem{TS: "1700000000.000000", UserName: "pat", Text: "parent-msg"}
	m.SetLastReadTS(replies[1].TS)

	m.SetThread(parent, replies[:3], "C1", parent.TS)
	if got := m.LastReadTS(); got != replies[1].TS {
		t.Errorf("refreshing the same thread changed the boundary: got %q, want %q", got, replies[1].TS)
	}

	m.SetThread(parent, replies, "C1", "1700000099.000000")
	if got := m.LastReadTS(); got != "" {
		t.Errorf("opening a different thread in the same channel kept the boundary %q", got)
	}

	m.SetLastReadTS(replies[1].TS)
	m.SetThread(parent, replies, "C2", "1700000099.000000")
	if got := m.LastReadTS(); got != "" {
		t.Errorf("opening a thread in a different channel kept the boundary %q", got)
	}
}
