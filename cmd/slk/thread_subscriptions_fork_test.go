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
	now := time.Now()
	claim, err := db.TryClaimThreadSweep("T1", now, 30*time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("seed claim: claim=%v err=%v", claim, err)
	}
	if err := claim.Complete(now); err != nil {
		t.Fatalf("seed claim Complete: %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("seed claim Close: %v", err)
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
// and pre-election every instance retried on its own gate. The failed
// sweep never completes its claim, so the next sibling takes it over.
func TestThreadSweep_FailedSweepLetsSiblingTakeOver(t *testing.T) {
	db := newTestDB(t)
	failing := &fakeSubscriptions{err: errors.New("network kaboom")}
	if err := newSubscriptionSync(db, failing, nil).syncIfUnclaimed(context.Background(), 30*time.Minute); err == nil {
		t.Fatal("failed sweep reported success")
	}

	if n := siblingSweeps(t, db); n != 1 {
		t.Fatalf("sibling swept %d times after our failure; want 1 — an uncompleted claim must be taken over", n)
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

	if n := siblingSweeps(t, db); n != 0 {
		t.Fatalf("sibling swept %d times right after ours completed; want 0", n)
	}
}

// A sweep whose context ends mid-paging is a failed sweep: the lister
// returns ctx.Err(), the claim is closed uncompleted, and the next
// sibling takes it over.
func TestThreadSweep_CancelledSweepLetsSiblingTakeOver(t *testing.T) {
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
		t.Fatalf("sibling swept %d times after our sweep was cancelled; want 1 — the claim must be taken over", n)
	}
}

// While a sweep is in flight its holder keeps the workspace's lock
// file, so a sibling attempting the same sweep does nothing; the
// lock, not a row, is what tells a live sweep from an abandoned one.
func TestThreadSweep_LiveSweepIsNotDuplicated(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	blocked := &blockingSubscriptions{entered: make(chan struct{})}
	s := &threadSubscriptionSync{client: blocked, db: db, workspaceID: "T1"}
	done := make(chan error, 1)
	go func() { done <- s.syncIfUnclaimed(ctx, 30*time.Minute) }()
	<-blocked.entered

	if n := siblingSweeps(t, db); n != 0 {
		t.Fatalf("sibling swept %d times while our sweep is in flight; want 0", n)
	}

	cancel()
	<-done
	if n := siblingSweeps(t, db); n != 1 {
		t.Fatalf("sibling swept %d times after our sweep was cancelled; want 1", n)
	}
}
