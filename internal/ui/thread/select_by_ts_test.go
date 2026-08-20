package thread

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

func threadFixture() (*Model, messages.MessageItem, []messages.MessageItem) {
	parent := messages.MessageItem{TS: "100.000001", Text: "parent"}
	replies := []messages.MessageItem{
		{TS: "200.000002", Text: "one"},
		{TS: "300.000003", Text: "two"},
		{TS: "400.000004", Text: "three"},
	}
	m := New()
	m.SetThread(parent, replies, "C1", parent.TS)
	return m, parent, replies
}

func TestSelectByTS(t *testing.T) {
	m, parent, _ := threadFixture()

	if !m.SelectByTS("300.000003") {
		t.Fatal("reply ts: want true")
	}
	if sel := m.SelectedReply(); sel == nil || sel.TS != "300.000003" {
		t.Errorf("reply ts: want 300.000003 selected, got %+v", sel)
	}
	if !m.SelectByTS(parent.TS) {
		t.Fatal("parent ts: want true")
	}
	if sel := m.SelectedReply(); sel == nil || sel.TS != parent.TS {
		t.Errorf("parent ts: want parent selected, got %+v", sel)
	}
	if m.SelectByTS("999.999999") {
		t.Error("unknown ts: want false")
	}
	if sel := m.SelectedReply(); sel == nil || sel.TS != parent.TS {
		t.Errorf("unknown ts: cursor must stay put, got %+v", sel)
	}
	if m.SelectByTS("") {
		t.Error("empty ts: want false")
	}
}

// TestPendingSelectPinsAcrossSetThread pins the permalink-open contract:
// SetThread runs more than once for the same thread (cache prime, then
// the authoritative fetch) and snaps the cursor to the newest reply
// each time; an armed pending-select ts re-selects the linked message
// on every pass, until the thread changes or the panel is cleared.
func TestPendingSelectPinsAcrossSetThread(t *testing.T) {
	m, parent, replies := threadFixture()
	m.SetPendingSelectTS("300.000003")

	for pass := 0; pass < 2; pass++ {
		m.SetThread(parent, replies, "C1", parent.TS)
		if sel := m.SelectedReply(); sel == nil || sel.TS != "300.000003" {
			t.Fatalf("pass %d: want pinned 300.000003, got %+v", pass, sel)
		}
	}

	// A different thread drops the pin and gets the default selection.
	other := messages.MessageItem{TS: "500.000005", Text: "other parent"}
	m.SetThread(other, replies, "C1", other.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != replies[len(replies)-1].TS {
		t.Errorf("new thread: want newest reply selected, got %+v", sel)
	}

	// Clear drops the pin too.
	m2, parent2, replies2 := threadFixture()
	m2.SetPendingSelectTS("300.000003")
	m2.Clear()
	m2.SetThread(parent2, replies2, "C1", parent2.TS)
	if sel := m2.SelectedReply(); sel == nil || sel.TS != replies2[len(replies2)-1].TS {
		t.Errorf("after Clear: want newest reply selected, got %+v", sel)
	}
}
