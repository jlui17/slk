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

// TestPendingSelectDisarmsOnceLanded pins the permalink-open contract:
// the pending-select pin selects the linked message on the first
// SetThread pass that contains it, then disarms — a later reload of the
// same thread (a reconnect refetch) must not yank the cursor back to
// the linked message once the user has moved it.
func TestPendingSelectDisarmsOnceLanded(t *testing.T) {
	m, parent, replies := threadFixture()
	m.SetPendingSelectTS("300.000003")

	m.SetThread(parent, replies, "C1", parent.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != "300.000003" {
		t.Fatalf("want pinned 300.000003, got %+v", sel)
	}

	// The user moves the cursor; a background reload keeps it there.
	m.SelectByTS("200.000002")
	m.SetThread(parent, replies, "C1", parent.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != "200.000002" {
		t.Errorf("reload after pin landed: want user's 200.000002, got %+v", sel)
	}

	// A different thread drops the pin and gets the default selection.
	m.SetPendingSelectTS("300.000003")
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

// TestPendingSelectSurvivesUntilTargetLoads: a permalink open runs
// SetThread in passes (stub, cache prime, authoritative fetch), and an
// early pass may not contain the linked message yet. The pin stays
// armed across a miss — and the cursor holds its place instead of
// snapping to the newest reply — until the pass that has the target.
func TestPendingSelectSurvivesUntilTargetLoads(t *testing.T) {
	parent := messages.MessageItem{TS: "100.000001", Text: "parent"}
	replies := []messages.MessageItem{
		{TS: "200.000002", Text: "one"},
		{TS: "300.000003", Text: "two"},
	}
	m := New()
	m.SetThread(parent, nil, "C1", parent.TS) // permalink stub open
	m.SetPendingSelectTS("300.000003")

	m.SetThread(parent, replies[:1], "C1", parent.TS) // prime without target
	if sel := m.SelectedReply(); sel == nil || sel.TS != parent.TS {
		t.Fatalf("pin miss: cursor must stay put (parent), got %+v", sel)
	}

	m.SetThread(parent, replies, "C1", parent.TS) // authoritative fetch
	if sel := m.SelectedReply(); sel == nil || sel.TS != "300.000003" {
		t.Errorf("want pinned 300.000003 once loaded, got %+v", sel)
	}
}

// TestReopenStubPassRestoresCursor: re-opening the already-open thread
// (openThreadPanel) runs a stub SetThread with nil replies before the
// replies reload lands. The stub pass can't find the cursor's message,
// so it arms the pin with it; the reload that has it puts the cursor
// back instead of leaving it on the parent.
func TestReopenStubPassRestoresCursor(t *testing.T) {
	m, parent, replies := threadFixture()
	m.SelectByTS("200.000002")

	m.SetThread(parent, nil, "C1", parent.TS)
	m.SetThread(parent, replies, "C1", parent.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != "200.000002" {
		t.Errorf("want cursor restored to 200.000002 after stub pass, got %+v", sel)
	}
}

// TestSameThreadReloadKeepsCursor: with no pin in play, a reload of the
// thread already on screen keeps the cursor on the message it was on
// (matched by ts, so appended replies don't move it), including the
// parent row, and keeps the snap state so the viewport doesn't jump.
// Only when the message is gone does the cursor fall back to the
// newest-reply default.
func TestSameThreadReloadKeepsCursor(t *testing.T) {
	m, parent, replies := threadFixture()
	m.SelectByTS("200.000002")
	m.hasSnapped = true
	m.snappedSelection = m.selected

	grown := append(append([]messages.MessageItem{}, replies...),
		messages.MessageItem{TS: "500.000005", Text: "four"})
	m.SetThread(parent, grown, "C1", parent.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != "200.000002" {
		t.Fatalf("reload: want cursor kept on 200.000002, got %+v", sel)
	}
	if !m.hasSnapped {
		t.Error("reload with unmoved cursor: want snap state kept so the viewport stays put")
	}

	// Cursor on the parent row survives a reload.
	m.SelectByTS(parent.TS)
	m.SetThread(parent, grown, "C1", parent.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != parent.TS {
		t.Errorf("reload on parent: want parent kept, got %+v", sel)
	}

	// The message under the cursor disappeared: fall back to newest.
	m.SelectByTS("200.000002")
	shrunk := []messages.MessageItem{{TS: "300.000003", Text: "two"}}
	m.SetThread(parent, shrunk, "C1", parent.TS)
	if sel := m.SelectedReply(); sel == nil || sel.TS != "300.000003" {
		t.Errorf("cursor message gone: want newest reply, got %+v", sel)
	}
}
