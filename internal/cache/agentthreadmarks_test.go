package cache

import "testing"

func TestAgentThreadMarkRoundtrip(t *testing.T) {
	db := newPaneStateTestDB(t)

	marked := func(ws, ch, ts string) bool {
		t.Helper()
		ok, err := db.AgentThreadMarked(ws, ch, ts)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if marked("T1", "C1", "100.0") {
		t.Fatal("empty table reads marked")
	}
	if err := db.MarkAgentThread("T1", "C1", "100.0"); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkAgentThread("T1", "C1", "100.0"); err != nil {
		t.Fatalf("marking twice: %v", err)
	}
	if !marked("T1", "C1", "100.0") {
		t.Fatal("marked thread reads unmarked")
	}
	// The key is the whole triple: a shared channel's same thread in
	// another workspace, or another thread in the channel, stays unmarked.
	if marked("T2", "C1", "100.0") || marked("T1", "C1", "200.0") || marked("T1", "C2", "100.0") {
		t.Fatal("mark leaked past its own thread")
	}
	if err := db.UnmarkAgentThread("T1", "C1", "100.0"); err != nil {
		t.Fatal(err)
	}
	if marked("T1", "C1", "100.0") {
		t.Fatal("unmarked thread reads marked")
	}
	if err := db.UnmarkAgentThread("T1", "C1", "100.0"); err != nil {
		t.Fatalf("unmarking an unmarked thread: %v", err)
	}
}
