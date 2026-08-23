package main

import (
	"time"

	"github.com/gammons/slk/internal/ui"
)

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

// sendCacheWatermark tells this instance's UI that everything the
// workspace cached before now needs revalidation on next open. See
// ui.CacheStaleBeforeMsg for why this replaced MarkChannelsStale: the
// cache.db is shared across instances, and zeroing synced_at there
// forced every sibling into a refetch storm on one instance's flap.
func (r *reconnectSync) sendCacheWatermark() {
	if r.program == nil {
		return
	}
	r.program.Send(ui.CacheStaleBeforeMsg{WorkspaceID: r.workspaceID, Before: time.Now()})
}
