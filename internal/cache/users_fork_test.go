package cache

import (
	"path/filepath"
	"testing"
)

// Every UpsertUser caller hardcodes Presence "away" (user resolution
// knows names and avatars, never live presence), so a re-resolution
// must not clobber the presence UpdatePresence recorded — the clobber
// used to self-heal via reconnect presence echoes, and the presence
// dedupe now (correctly) swallows those.
func TestUpsertUser_DoesNotClobberLivePresence(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "u.db"))
	if err := db.UpsertWorkspace(Workspace{ID: "T1", Name: "T"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice", Presence: "away"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdatePresence("U1", "active"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice-renamed", Presence: "away"}); err != nil {
		t.Fatal(err)
	}
	u, err := db.GetUser("U1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Presence != "active" {
		t.Fatalf("presence = %q after re-resolution; want the live \"active\" preserved", u.Presence)
	}
	if u.Name != "alice-renamed" {
		t.Fatalf("name = %q; the rest of the upsert must still apply", u.Name)
	}
}

func TestUpsertUser_InsertStillSeedsPresence(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "u.db"))
	if err := db.UpsertWorkspace(Workspace{ID: "T1", Name: "T"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice", Presence: "away"}); err != nil {
		t.Fatal(err)
	}
	u, err := db.GetUser("U1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Presence != "away" {
		t.Fatalf("presence = %q on first insert; want the seeded \"away\"", u.Presence)
	}
}
