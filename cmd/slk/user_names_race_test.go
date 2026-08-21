// cmd/slk/user_names_race_test.go
//
// Regression guard for a fatal concurrent-map throw, not a style or
// speed concern: before the usernames.Store existed, the WS goroutine's
// resolveUserCached memoization wrote the same plain map the UI
// goroutine reads while rendering, and Go kills the process on a
// concurrent map read+write (no recover). This test hammers that exact
// pair — resolveUserCached as the RTM/WebSocket inbound-message path
// calls it, against render-shaped reads — and MUST run under -race to
// mean anything. Do not delete it as slow; it is what stands between
// the store and a silent reintroduction of the crash.
package main

import (
	"fmt"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/usernames"
)

func TestUserNamesConcurrentResolveAndRead(t *testing.T) {
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer db.Close()
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T1", Name: "Test"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	const users = 200
	for i := 0; i < users; i++ {
		id := fmt.Sprintf("U%04d", i)
		if err := db.UpsertUser(cache.User{ID: id, WorkspaceID: "T1", Name: id, DisplayName: "User " + id}); err != nil {
			t.Fatalf("UpsertUser %s: %v", id, err)
		}
	}

	names := usernames.NewStore()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < users; i++ {
			// Every ID is a store miss with a DB hit, so each call takes
			// the memoizing-write path — the write OnMessage performs.
			resolveUserCached(fmt.Sprintf("U%04d", i), names, db)
		}
	}()
	for {
		select {
		case <-done:
			for i := 0; i < users; i++ {
				id := fmt.Sprintf("U%04d", i)
				if name, ok := names.Get(id); !ok || name != "User "+id {
					t.Fatalf("names.Get(%s) = (%q, %v), want (\"User %s\", true)", id, name, ok, id)
				}
			}
			return
		default:
		}
		current := names.Current()
		for i := 0; i < users; i++ {
			_ = current[fmt.Sprintf("U%04d", i)]
		}
	}
}
