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
