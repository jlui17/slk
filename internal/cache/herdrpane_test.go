package cache

import "testing"

func TestHerdrPaneIDRoundtripAndOverwrite(t *testing.T) {
	db := newPaneStateTestDB(t)

	if _, ok, err := db.GetHerdrPaneID("w1:p1"); err != nil || ok {
		t.Fatalf("empty table: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}

	if err := db.RecordHerdrPaneID("w1:p1", "w2:p2"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetHerdrPaneID("w1:p1")
	if err != nil || !ok {
		t.Fatalf("GetHerdrPaneID: ok=%v err=%v", ok, err)
	}
	if got != "w2:p2" {
		t.Errorf("got %q, want %q", got, "w2:p2")
	}

	// A later move overwrites the same key.
	if err := db.RecordHerdrPaneID("w1:p1", "w3:p1"); err != nil {
		t.Fatal(err)
	}
	got, ok, err = db.GetHerdrPaneID("w1:p1")
	if err != nil || !ok {
		t.Fatalf("GetHerdrPaneID after overwrite: ok=%v err=%v", ok, err)
	}
	if got != "w3:p1" {
		t.Errorf("got %q, want %q", got, "w3:p1")
	}
}

func TestHerdrPaneIDKeyIsolation(t *testing.T) {
	db := newPaneStateTestDB(t)

	if err := db.RecordHerdrPaneID("w1:p1", "w2:p2"); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordHerdrPaneID("w1:p2", "w4:p1"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetHerdrPaneID("w1:p1")
	if err != nil || !ok {
		t.Fatalf("GetHerdrPaneID w1:p1: ok=%v err=%v", ok, err)
	}
	if got != "w2:p2" {
		t.Errorf("w1:p1 mapping clobbered by w1:p2 write: %q", got)
	}
	got, ok, err = db.GetHerdrPaneID("w1:p2")
	if err != nil || !ok {
		t.Fatalf("GetHerdrPaneID w1:p2: ok=%v err=%v", ok, err)
	}
	if got != "w4:p1" {
		t.Errorf("w1:p2 mapping = %q, want %q", got, "w4:p1")
	}
	// A key the table has never seen must miss even when rows exist: a
	// key-blind read would hand a cold start some other pane's identity.
	if _, ok, err := db.GetHerdrPaneID("w9:p9"); err != nil || ok {
		t.Errorf("unknown key: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

func TestHerdrTabLabelRoundtripAndOverwrite(t *testing.T) {
	db := newPaneStateTestDB(t)

	if _, ok, err := db.GetHerdrTabLabel("w1:p1"); err != nil || ok {
		t.Fatalf("empty table: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}

	if err := db.RecordHerdrTabLabel("w1:p1", "fix retries"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetHerdrTabLabel("w1:p1")
	if err != nil || !ok || got != "fix retries" {
		t.Fatalf("GetHerdrTabLabel: got %q ok=%v err=%v", got, ok, err)
	}

	if err := db.RecordHerdrTabLabel("w1:p1", "[colony-562] fix flow viewer"); err != nil {
		t.Fatal(err)
	}
	got, ok, err = db.GetHerdrTabLabel("w1:p1")
	if err != nil || !ok || got != "[colony-562] fix flow viewer" {
		t.Fatalf("after overwrite: got %q ok=%v err=%v", got, ok, err)
	}
}
