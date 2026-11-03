package slackclient

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/gammons/slk/internal/debuglog"
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

// settleBackoff runs after a socket ends. It resets the backoff and
// the attempt count only if the socket proved healthy, so a socket
// Slack accepts and then rejects counts as a failed attempt.
func (cm *ConnectionManager) settleBackoff(backoff time.Duration, attempt int) (time.Duration, int) {
	o := cm.client.wsOutcome
	now := time.Now()
	healthy := o.provedHealthy(now)
	debuglog.WS("team=%s socket ended: hello=%v healthy=%v error=%q retry_after=%v up=%v read_err=%v pings=%d pongs=%d",
		cm.client.teamID, !o.helloAt.IsZero(), healthy, o.errorMsg, o.retryAfter,
		now.Sub(o.openedAt).Round(time.Millisecond), o.readErr, o.pings.Load(), o.pongs.Load())
	if healthy {
		return 1 * time.Second, 0
	}
	return backoff, attempt
}

// reconnectWait is how long Run waits before the next dial: a random
// point in the upper half of backoff, so slks that lost their sockets
// together don't redial together, but never less than the retryAfter
// Slack asked for.
func reconnectWait(backoff, retryAfter time.Duration) time.Duration {
	half := backoff / 2
	return max(half+rand.N(half+1), retryAfter)
}

// waitBackoff reports the wait via OnReconnectWait, then sleeps for
// wait. ok is false when ctx was cancelled; kicked is true when a
// Reconnect kick ended (or pre-empted) the wait. A kick already
// pending on entry (a manual reload just closed the socket) returns
// immediately WITHOUT reporting the wait: no wait happens, so
// reporting one would flash a bogus countdown in the UI.
func (cm *ConnectionManager) waitBackoff(ctx context.Context, wait time.Duration, attempt int) (kicked, ok bool) {
	select {
	case <-cm.kick:
		return true, true
	default:
	}
	debuglog.WS("team=%s reconnect wait=%v attempt=%d", cm.client.teamID, wait.Round(time.Millisecond), attempt)
	if cm.OnReconnectWait != nil {
		cm.OnReconnectWait(time.Now().Add(wait), attempt)
	}
	select {
	case <-ctx.Done():
		return false, false
	case <-cm.kick:
		return true, true
	case <-time.After(wait):
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
