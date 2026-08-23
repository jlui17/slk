package main

import (
	"testing"
	"time"
)

// TestDedupeGateReset pins the manual-reload contract: reset clears
// the window so the next pass runs immediately.
func TestDedupeGateReset(t *testing.T) {
	g := dedupeGate{window: 30 * time.Second}
	now := time.Now()
	if !g.tryStart(now) {
		t.Fatal("first tryStart must succeed")
	}
	if g.tryStart(now.Add(time.Second)) {
		t.Fatal("tryStart inside the window must be suppressed")
	}
	g.reset()
	if !g.tryStart(now.Add(2 * time.Second)) {
		t.Fatal("tryStart after reset must succeed")
	}
}
