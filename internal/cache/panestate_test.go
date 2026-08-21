package cache

import "testing"

func newPaneStateTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPaneStateRoundtrip(t *testing.T) {
	db := newPaneStateTestDB(t)

	if _, ok, err := db.GetPaneState("p1"); err != nil || ok {
		t.Fatalf("empty table: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}

	want := PaneState{WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "111.222"}
	if err := db.RecordPaneState("p1", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetPaneState("p1")
	if err != nil || !ok {
		t.Fatalf("GetPaneState: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPaneStateUpsertAndThreadClear(t *testing.T) {
	db := newPaneStateTestDB(t)

	if err := db.RecordPaneState("p1", PaneState{WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "111.222"}); err != nil {
		t.Fatal(err)
	}
	// Closing the thread records the same channel with no thread.
	want := PaneState{WorkspaceID: "T1", ChannelID: "C1"}
	if err := db.RecordPaneState("p1", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetPaneState("p1")
	if err != nil || !ok {
		t.Fatalf("GetPaneState: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPaneStateKeyIsolation(t *testing.T) {
	db := newPaneStateTestDB(t)

	if err := db.RecordPaneState("p1", PaneState{WorkspaceID: "T1", ChannelID: "C1"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordPaneState("p2", PaneState{WorkspaceID: "T2", ChannelID: "C2", ThreadTS: "333.444"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetPaneState("p1")
	if err != nil || !ok {
		t.Fatalf("GetPaneState p1: ok=%v err=%v", ok, err)
	}
	if got.ChannelID != "C1" || got.ThreadTS != "" {
		t.Errorf("p1 state clobbered by p2 write: %+v", got)
	}
}
