package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
