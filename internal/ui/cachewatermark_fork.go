package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// CacheStaleBeforeMsg marks everything a workspace cached before the
// given instant as needing revalidation on next open. Sent by this
// instance's reconnect catch-up instead of zeroing synced_at in the
// shared cache.db: the DB is shared by every running slk instance, and
// a sibling whose socket stayed live kept the cache genuinely fresh —
// its synced_at writes must keep counting, both for it and for us.
type CacheStaleBeforeMsg struct {
	WorkspaceID string
	Before      time.Time
}

var reduceCacheWatermark reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(CacheStaleBeforeMsg)
	if !ok {
		return nil, false
	}
	if a.cacheStaleBefore == nil {
		a.cacheStaleBefore = make(map[string]time.Time)
	}
	if m.Before.After(a.cacheStaleBefore[m.WorkspaceID]) {
		a.cacheStaleBefore[m.WorkspaceID] = m.Before
	}
	return nil, true
}

// syncedAfterWatermark reports whether a synced_at instant postdates
// the active workspace's reconnect watermark. A missing watermark (nil
// map or unmarked workspace) reads as the zero time, so everything
// passes until a reconnect sets one.
func (a *App) syncedAfterWatermark(syncedAt int64) bool {
	return time.Unix(syncedAt, 0).After(a.cacheStaleBefore[a.activeTeamID])
}
