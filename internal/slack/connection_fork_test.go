package slackclient

import (
	"context"
	"testing"
	"time"
)

// TestReconnectSkipsBackoffWait pins the manual-reload contract: a
// Reconnect kick ends a pending backoff wait immediately instead of
// sleeping it out.
func TestReconnectSkipsBackoffWait(t *testing.T) {
	cm := NewConnectionManager(&Client{}, nil)
	cm.Reconnect()

	reported := false
	cm.OnReconnectWait = func(time.Time, int) { reported = true }

	type result struct{ kicked, ok bool }
	done := make(chan result, 1)
	go func() {
		kicked, ok := cm.waitBackoff(context.Background(), time.Hour, 1)
		done <- result{kicked, ok}
	}()
	select {
	case r := <-done:
		if !r.ok {
			t.Fatal("waitBackoff returned ok=false without ctx cancellation")
		}
		if !r.kicked {
			t.Fatal("waitBackoff must report kicked=true when ended by a Reconnect kick")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitBackoff did not return after a Reconnect kick")
	}
	if reported {
		t.Fatal("a kicked wait must not report a reconnect countdown: no wait happens")
	}
}

// TestAdjustBackoff pins the manual-reload recovery contract: a kicked
// wait resets backoff to 1s (the kick consumed no real time, so
// escalating would walk the backoff to its cap in seconds and delay
// the next automatic retry); a timed-out wait escalates as before.
func TestAdjustBackoff(t *testing.T) {
	if got := adjustBackoff(true, 16*time.Second, 30*time.Second); got != 1*time.Second {
		t.Fatalf("kicked wait: backoff = %v, want 1s", got)
	}
	if got := adjustBackoff(false, 16*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("timed-out wait: backoff = %v, want 30s", got)
	}
}

func TestWaitBackoffReportsWait(t *testing.T) {
	cm := NewConnectionManager(&Client{}, nil)
	var gotAttempt int
	var gotRetryAt time.Time
	cm.OnReconnectWait = func(retryAt time.Time, attempt int) {
		gotRetryAt = retryAt
		gotAttempt = attempt
	}

	before := time.Now()
	kicked, ok := cm.waitBackoff(context.Background(), 10*time.Millisecond, 3)
	if !ok {
		t.Fatal("waitBackoff returned ok=false without ctx cancellation")
	}
	if kicked {
		t.Fatal("waitBackoff reported kicked=true with no Reconnect kick")
	}
	if gotAttempt != 3 {
		t.Fatalf("attempt = %d, want 3", gotAttempt)
	}
	if gotRetryAt.Before(before) {
		t.Fatalf("retryAt %v is before the wait started %v", gotRetryAt, before)
	}
}

func TestWaitBackoffCancelled(t *testing.T) {
	cm := NewConnectionManager(&Client{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := cm.waitBackoff(ctx, time.Hour, 1); ok {
		t.Fatal("waitBackoff returned ok=true on a cancelled ctx")
	}
}
