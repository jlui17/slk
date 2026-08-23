package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
)

func TestHerdrPaneIDStoreKeysByLaunchID(t *testing.T) {
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	load, save := herdrPaneIDStore(db, "w1:p1")

	if _, ok := load(); ok {
		t.Fatal("empty store must miss")
	}
	if err := save("w2:p2"); err != nil {
		t.Fatal(err)
	}
	if got, ok := load(); !ok || got != "w2:p2" {
		t.Fatalf("load = %q, %v; want %q, true", got, ok, "w2:p2")
	}
	// Both hook parameters are pane-id-shaped strings, so a load/save
	// roundtrip alone also passes with the key and value transposed;
	// pin that the row is keyed by the launch id, not the resolved one.
	if got, ok, err := db.GetHerdrPaneID("w1:p1"); err != nil || !ok || got != "w2:p2" {
		t.Errorf("row under launch id: %q, %v, %v; want %q, true, nil", got, ok, err, "w2:p2")
	}
	if _, ok, _ := db.GetHerdrPaneID("w2:p2"); ok {
		t.Error("row keyed by the resolved id instead of the launch id")
	}
}
