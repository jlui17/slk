package slackclient

import (
	"context"
	"time"
)

// ConnectionManager manages the WebSocket connection lifecycle with
// automatic reconnection using exponential backoff.
type ConnectionManager struct {
	client  *Client
	handler EventHandler
	cancel  context.CancelFunc
	kick    chan struct{}

	// OnReconnectWait, when non-nil, is called at the start of each
	// backoff wait with the redial deadline and the attempt number
	// (1-based, reset on every successful connect). Set before Run;
	// it is invoked from Run's goroutine and must not block.
	OnReconnectWait func(retryAt time.Time, attempt int)
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(client *Client, handler EventHandler) *ConnectionManager {
	return &ConnectionManager{
		client:  client,
		handler: handler,
		kick:    make(chan struct{}, 1),
	}
}

// Run starts the connection loop. It connects, waits for disconnect,
// and reconnects with exponential backoff. Blocks until ctx is cancelled.
func (cm *ConnectionManager) Run(ctx context.Context) {
	ctx, cm.cancel = context.WithCancel(ctx)

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := cm.client.StartWebSocket(cm.handler)
		if err != nil {
			cm.handler.OnDisconnect()
			attempt++
			kicked, ok := cm.waitBackoff(ctx, backoff, attempt)
			if !ok {
				return
			}
			backoff = adjustBackoff(kicked, backoff, maxBackoff)
			continue
		}

		// Connected — reset backoff. A kick left pending from the dial
		// window is deliberately NOT drained: Reconnect() pairs the kick
		// with a socket close, so discarding it here would swallow a
		// manual reload that landed between connect and this line (the
		// close still tears the socket down, and the loop would then
		// sleep out a backoff the user asked to skip). The cost of
		// keeping a genuinely stale kick is one skipped wait plus a
		// backoff reset after an explicit user gesture — the same
		// "reconnect now" the gesture meant.
		backoff = 1 * time.Second
		attempt = 0

		// Wait for disconnect
		select {
		case <-ctx.Done():
			cm.client.StopWebSocket()
			return
		case <-cm.client.WsDone():
			// Disconnected — will reconnect after backoff
		}

		attempt++
		kicked, ok := cm.waitBackoff(ctx, backoff, attempt)
		if !ok {
			return
		}
		backoff = adjustBackoff(kicked, backoff, maxBackoff)
	}
}

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

// Stop cancels the connection loop and closes the WebSocket.
func (cm *ConnectionManager) Stop() {
	if cm.cancel != nil {
		cm.cancel()
	}
	cm.client.StopWebSocket()
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
