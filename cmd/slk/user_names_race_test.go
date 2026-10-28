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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/threadsview"
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

// TestFetchThreadReplies_RacesThreadsViewRender is upstream's repro of
// its 2026-10-02 crash ("fatal error: concurrent map read and map
// write", top frames messages.renderInlineFormattingWith <-
// threadsview.renderCard), wired the fork's way: the Threads view and
// fetchThreadReplies share one usernames.Store, the fetcher (a
// bubbletea Cmd goroutine) memoizing a SQLite-found reply author into
// it while the view renders from it. Upstream fixed the crash with an
// engine-only locked store and UI snapshots; the fork's store is
// already safe to share. Run with -race.
func TestFetchThreadReplies_RacesThreadsViewRender(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertUser(cache.User{ID: "U2", WorkspaceID: "T1", Name: "bob", DisplayName: "Bob"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"has_more":false,"messages":[
			{"type":"message","user":"U1","ts":"1700000000.000100","thread_ts":"1700000000.000100","text":"parent"},
			{"type":"message","user":"U2","ts":"1700000001.000100","thread_ts":"1700000000.000100","text":"reply"}]}`))
	}))
	defer srv.Close()
	client := newTestClient(t, srv)

	store := usernames.FromMap(map[string]string{"U1": "Alice"})

	tv := threadsview.New(nil, "USELF")
	tv.SetUserNames(store)
	tv.SetSummaries([]core.ThreadSummary{{
		ChannelID: "C1", ThreadTS: "1700000000.000100", ParentTS: "1700000000.000100",
		ParentUserID: "U1", ParentText: "ping <@U1> and <@U2>",
	}})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() { // the UI goroutine, rendering
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = tv.View(20, 80)
			}
		}
	}()

	// The bubbletea Cmd goroutine, fetching the selected thread.
	replies := fetchThreadReplies(client, "C1", "1700000000.000100", db, store, "15:04", nil, nil)
	close(stop)
	wg.Wait()

	if name, ok := store.Get("U2"); !ok || name != "Bob" {
		t.Errorf("store.Get(U2) = (%q, %v), want (\"Bob\", true)", name, ok)
	}
	if len(replies) != 1 || replies[0].UserName != "Bob" {
		t.Errorf("replies = %+v, want one reply by Bob", replies)
	}
}
