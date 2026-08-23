package main

import "time"

// reset clears the gate so the next tryStart succeeds regardless of
// when the last pass ran. Used by the manual reload, where explicit
// user intent outranks flap protection.
func (g *dedupeGate) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.last = time.Time{}
}

// ForceReconnect bounces this workspace's websocket immediately AND
// resets the reconnect catch-up dedupe gate so the catch-up pass runs
// on the resulting OnConnect. Always use this over ConnMgr.Reconnect()
// directly: a bare Reconnect inside the gate's dedupe window silently
// skips the catch-up (debug-log only), leaving offline-gap messages
// unbackfilled.
func (w *WorkspaceContext) ForceReconnect() {
	w.RTMHandler.backfillGate.reset()
	w.ConnMgr.Reconnect()
}
