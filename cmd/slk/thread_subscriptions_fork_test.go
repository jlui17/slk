package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
)

// blockingSubscriptions is a getView walk caught mid-paging: it signals
// entered, then blocks until its context ends, as the real lister's
// HTTP call does.
type blockingSubscriptions struct {
	entered chan struct{}
}

func (b *blockingSubscriptions) ListThreadSubscriptions(ctx context.Context) ([]slackclient.ThreadSubscriptionView, error) {
	close(b.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

// siblingSweeps runs a second instance's sweep attempt against the
// same DB and reports how many getView walks it made.
func siblingSweeps(t *testing.T, db *cache.DB) int {
	t.Helper()
	sibling := &fakeSubscriptions{}
	if err := newSubscriptionSync(db, sibling, nil).syncIfUnclaimed(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("sibling syncIfUnclaimed: %v", err)
	}
	return sibling.calls
}

// Ten instances share one cache.db and the sweep writes everything it
// learns into that DB, so a sibling's recent sweep substitutes for
// ours: skip the ~62-request getView walk when the shared claim is
// fresh, and let the caller's onDone re-read the shared cache.
func TestThreadSweep_SkipsWhenSiblingClaimedRecently(t *testing.T) {
	db := newTestDB(t)
	claimed, err := db.TryClaimThreadSweep("T1", time.Now(), 30*time.Minute)
	if err != nil || !claimed {
		t.Fatalf("seed claim: claimed=%v err=%v", claimed, err)
	}

	fake := &fakeSubscriptions{}
	s := newSubscriptionSync(db, fake, nil)
	if err := s.syncIfUnclaimed(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("syncIfUnclaimed: %v", err)
	}
	if fake.calls != 0 {
		t.Fatalf("ListThreadSubscriptions called %d times behind a sibling's fresh claim; want 0", fake.calls)
	}
}

// A transient getView failure must not burn the fleet-wide claim for
// the whole window: reconnect/wake is exactly when errors are likely,
// and pre-election every instance retried on its own gate.
func TestThreadSweep_FailedSweepReleasesTheClaim(t *testing.T) {
	db := newTestDB(t)
	failing := &fakeSubscriptions{err: errors.New("network kaboom")}
	if err := newSubscriptionSync(db, failing, nil).syncIfUnclaimed(context.Background(), 30*time.Minute); err == nil {
		t.Fatal("failed sweep reported success")
	}

	sibling := &fakeSubscriptions{}
	if err := newSubscriptionSync(db, sibling, nil).syncIfUnclaimed(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("sibling syncIfUnclaimed: %v", err)
	}
	if sibling.calls != 1 {
		t.Fatalf("sibling swept %d times after our failure; want 1 — the failed claim must be released", sibling.calls)
	}
}

func TestThreadSweep_ClaimsAndRunsWhenUnclaimed(t *testing.T) {
	db := newTestDB(t)
	fake := &fakeSubscriptions{}
	s := newSubscriptionSync(db, fake, nil)
	if err := s.syncIfUnclaimed(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("syncIfUnclaimed: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("ListThreadSubscriptions called %d times on an unclaimed workspace; want 1", fake.calls)
	}

	sibling := &fakeSubscriptions{}
	s2 := newSubscriptionSync(db, sibling, nil)
	if err := s2.syncIfUnclaimed(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("sibling syncIfUnclaimed: %v", err)
	}
	if sibling.calls != 0 {
		t.Fatalf("sibling swept %d times right after ours claimed; want 0", sibling.calls)
	}
}

// A sweep whose context ends mid-paging is a failed sweep: the lister
// returns ctx.Err() and the release, which takes no context, still
// lands.
func TestThreadSweep_CancelledSweepReleasesTheClaim(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	blocked := &blockingSubscriptions{entered: make(chan struct{})}
	s := &threadSubscriptionSync{client: blocked, db: db, workspaceID: "T1"}
	done := make(chan error, 1)
	go func() { done <- s.syncIfUnclaimed(ctx, 30*time.Minute) }()
	<-blocked.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled sweep returned %v; want context.Canceled", err)
	}

	if n := siblingSweeps(t, db); n != 1 {
		t.Fatalf("sibling swept %d times after our sweep was cancelled; want 1 — the claim must be released", n)
	}
}

// Quit cancels nothing: both trigger sites hand the sweep
// context.Background(), so a sweep in flight when the UI loop exits
// dies with the process, having written nothing. Its claim must not
// outlive it, or every instance skips the sweep for the whole window.
func TestThreadSweep_QuitMidSweepReleasesTheClaim(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	blocked := &blockingSubscriptions{entered: make(chan struct{})}
	s := &threadSubscriptionSync{client: blocked, db: db, workspaceID: "T1"}
	done := make(chan error, 1)
	go func() { done <- s.syncIfUnclaimed(ctx, 30*time.Minute) }()
	t.Cleanup(func() { cancel(); <-done })
	<-blocked.entered

	releaseInFlightThreadSweepClaims(db)

	if n := siblingSweeps(t, db); n != 1 {
		t.Fatalf("sibling swept %d times after we quit mid-sweep; want 1 — the claim must be released", n)
	}
}

// A completed sweep's claim is what spares the siblings their own
// sweep for the window; quitting afterwards must leave it standing.
func TestThreadSweep_QuitAfterCompletedSweepKeepsTheClaim(t *testing.T) {
	db := newTestDB(t)
	if err := newSubscriptionSync(db, &fakeSubscriptions{}, nil).syncIfUnclaimed(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("syncIfUnclaimed: %v", err)
	}

	releaseInFlightThreadSweepClaims(db)

	if n := siblingSweeps(t, db); n != 0 {
		t.Fatalf("sibling swept %d times after we completed a sweep and quit; want 0", n)
	}
}
