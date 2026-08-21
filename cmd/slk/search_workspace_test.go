// cmd/slk/search_workspace_test.go
//
// Tests for the SearchWorkspace service closure wiring.
package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/usernames"
)

// TestSearchWorkspaceNoActiveWorkspaceReturnsErr verifies the
// SearchWorkspace closure never returns a nil msg when no workspace
// is active: a nil msg would leave the ctrl+f modal spinner stuck
// (the reducer only clears loading on a WorkspaceSearchResultsMsg).
// TestLookupUserCachedDoesNotMutateMap pins the read-only contract of
// lookupUserCached: a DB hit must be returned WITHOUT being memoized
// into the store (memoization is resolveUserCached's job; the search
// path deliberately skips it).
func TestLookupUserCachedDoesNotMutateMap(t *testing.T) {
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer db.Close()
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T1", Name: "Test"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	if err := db.UpsertUser(cache.User{ID: "U1", WorkspaceID: "T1", Name: "alice", DisplayName: "Alice"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	userNames := usernames.NewStore()
	name, ok := lookupUserCached("U1", userNames, db)
	if !ok || name != "Alice" {
		t.Fatalf("lookupUserCached = (%q, %v), want (\"Alice\", true)", name, ok)
	}
	if userNames.Len() != 0 {
		t.Fatalf("lookupUserCached mutated the store: %v", userNames.Current())
	}

	// Store hits still work and still don't publish.
	userNames = usernames.FromMap(map[string]string{"U2": "Bob"})
	v0 := userNames.Version()
	name, ok = lookupUserCached("U2", userNames, db)
	if !ok || name != "Bob" {
		t.Fatalf("store hit = (%q, %v), want (\"Bob\", true)", name, ok)
	}
	if userNames.Len() != 1 || userNames.Version() != v0 {
		t.Fatalf("store changed: %v", userNames.Current())
	}
}

func TestSearchWorkspaceNoActiveWorkspaceReturnsErr(t *testing.T) {
	fn := searchWorkspaceFunc(newWorkspaceRouter(), nil, "15:04")
	msg := fn("deploy")
	res, ok := msg.(ui.WorkspaceSearchResultsMsg)
	if !ok {
		t.Fatalf("got %T, want ui.WorkspaceSearchResultsMsg", msg)
	}
	if res.Query != "deploy" || res.Err == nil {
		t.Fatalf("res = %+v, want Query=%q and non-nil Err", res, "deploy")
	}
}
