package slackclient

import (
	"context"
	"time"
)

// adjustBackoff picks the next backoff after a wait. A kicked wait
// (manual reload) resets to 1s rather than escalating: the kick skips
// the sleep, so escalating would walk the backoff to its 30s cap in
// near-zero real time and leave the next automatic retry waiting the
// full cap — a manual "reconnect now" must never slow recovery down.
func adjustBackoff(kicked bool, current, max time.Duration) time.Duration {
	if kicked {
		return 1 * time.Second
	}
	return nextBackoff(current, max)
}

// waitBackoff reports the wait via OnReconnectWait, then sleeps for
// backoff. ok is false when ctx was cancelled; kicked is true when a
// Reconnect kick ended (or pre-empted) the wait. A kick already
// pending on entry (a manual reload just closed the socket) returns
// immediately WITHOUT reporting the wait: no wait happens, so
// reporting one would flash a bogus countdown in the UI.
func (cm *ConnectionManager) waitBackoff(ctx context.Context, backoff time.Duration, attempt int) (kicked, ok bool) {
	select {
	case <-cm.kick:
		return true, true
	default:
	}
	if cm.OnReconnectWait != nil {
		cm.OnReconnectWait(time.Now().Add(backoff), attempt)
	}
	select {
	case <-ctx.Done():
		return false, false
	case <-cm.kick:
		return true, true
	case <-time.After(backoff):
		return false, true
	}
}

// Reconnect forces an immediate redial: any pending backoff wait is
// skipped and the current socket (if up) is closed, which makes Run
// redial right away. Safe to call in any state. The reconnect
// catch-up's dedupe gate lives outside this package and is NOT reset
// here — callers that need the catch-up guaranteed pair this with a
// gate reset (see WorkspaceContext.ForceReconnect in cmd/slk).
func (cm *ConnectionManager) Reconnect() {
	select {
	case cm.kick <- struct{}{}:
	default:
	}
	cm.client.StopWebSocket()
}
