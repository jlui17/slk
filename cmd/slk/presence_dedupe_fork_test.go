package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui"
)

func TestPresenceDedupe_Changed(t *testing.T) {
	var d presenceDedupe
	if !d.Changed("U1", "active") {
		t.Error("first sighting should report changed")
	}
	if d.Changed("U1", "active") {
		t.Error("repeat of same value should not report changed")
	}
	if !d.Changed("U1", "away") {
		t.Error("transition to new value should report changed")
	}
	if !d.Changed("U2", "away") {
		t.Error("first sighting of another user should report changed")
	}
}

func TestOnPresenceChange_FirstSightingWritesAndSends(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertUser(cache.User{ID: "U1", WorkspaceID: "T1", Name: "u1", Presence: "away"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnPresenceChange("U1", "active")

	u, err := db.GetUser("U1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Presence != "active" {
		t.Errorf("Presence = %q, want %q", u.Presence, "active")
	}
	msgs := sentOfType[ui.PresenceChangeMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("sent %d PresenceChangeMsg, want 1", len(msgs))
	}
	if msgs[0].UserID != "U1" || msgs[0].Presence != "active" {
		t.Errorf("msg = %+v, want UserID=U1 Presence=active", msgs[0])
	}
}

func TestOnPresenceChange_UnchangedSkipsDBWriteAndSend(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertUser(cache.User{ID: "U1", WorkspaceID: "T1", Name: "u1"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnPresenceChange("U1", "active")
	// Poison the row behind the handler's back: a deduped repeat must
	// not touch the DB, so the poison value survives.
	if err := db.UpdatePresence("U1", "poisoned"); err != nil {
		t.Fatalf("UpdatePresence: %v", err)
	}
	h.OnPresenceChange("U1", "active")

	u, err := db.GetUser("U1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Presence != "poisoned" {
		t.Errorf("Presence = %q, want %q (repeat event must not write DB)", u.Presence, "poisoned")
	}
	if msgs := sentOfType[ui.PresenceChangeMsg](sender); len(msgs) != 1 {
		t.Errorf("sent %d PresenceChangeMsg, want 1 (repeat event must not send)", len(msgs))
	}
}

func TestOnPresenceChange_TransitionPassesEachStep(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertUser(cache.User{ID: "U1", WorkspaceID: "T1", Name: "u1"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnPresenceChange("U1", "active")
	h.OnPresenceChange("U1", "away")
	h.OnPresenceChange("U1", "active")

	msgs := sentOfType[ui.PresenceChangeMsg](sender)
	if len(msgs) != 3 {
		t.Fatalf("sent %d PresenceChangeMsg, want 3", len(msgs))
	}
	want := []string{"active", "away", "active"}
	for i, w := range want {
		if msgs[i].Presence != w {
			t.Errorf("msgs[%d].Presence = %q, want %q", i, msgs[i].Presence, w)
		}
	}
	u, err := db.GetUser("U1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Presence != "active" {
		t.Errorf("Presence = %q, want %q", u.Presence, "active")
	}
}
