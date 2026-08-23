package main

import (
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/usernames"
)

// CustomEmoji returns this workspace's emoji name -> URL (or
// "alias:target") map, or an empty map before any fetch has published
// one. Safe to call from any goroutine; the result must be treated as
// read-only.
func (w *WorkspaceContext) CustomEmoji() map[string]string {
	if m := w.customEmoji.Load(); m != nil {
		return *m
	}
	return map[string]string{}
}

// SetCustomEmoji publishes an emoji map for this workspace. The caller
// must not mutate the map afterwards.
func (w *WorkspaceContext) SetCustomEmoji(emojis map[string]string) {
	w.customEmoji.Store(&emojis)
}

func (r *workspaceRouter) Add(wctx *WorkspaceContext) {
	r.allMu.Lock()
	defer r.allMu.Unlock()
	r.all[wctx.TeamID] = wctx
}

// All returns a snapshot of every connected workspace.
func (r *workspaceRouter) All() []*WorkspaceContext {
	r.allMu.RLock()
	defer r.allMu.RUnlock()
	out := make([]*WorkspaceContext, 0, len(r.all))
	for _, wctx := range r.all {
		out = append(out, wctx)
	}
	return out
}

// userNameFill batches one fetch/load pass's memoizations so the store
// publishes once per pass (one copy-on-write) instead of once per
// author.
type userNameFill struct {
	names   *usernames.Store
	pending map[string]string
}

func newUserNameFill(names *usernames.Store) *userNameFill {
	return &userNameFill{names: names, pending: map[string]string{}}
}

// lookup is lookupUserCached with this pass's pending fills consulted
// first and DB hits (only) memoized into them — store hits need no
// republish, and apply's Fill keeps a pending name from overwriting a
// fresher live-resolved one.
func (f *userNameFill) lookup(userID string, db *cache.DB) (string, bool) {
	if name, ok := f.pending[userID]; ok && name != "" {
		return name, true
	}
	if name, ok := f.names.Get(userID); ok && name != "" {
		return name, true
	}
	name, ok := lookupUserCached(userID, f.names, db)
	if ok {
		f.pending[userID] = name
	}
	return name, ok
}

func (f *userNameFill) apply() {
	f.names.Fill(f.pending)
}

// threadMarkReadState decides what a thread_marked watermark means for
// read state. Equality means caught up: a genuine mark-as-unread sets
// last_read strictly before the message being marked, so last_read can
// only reach the newest activity by reading to the end. A watermark past
// everything known (or nothing known at all) is undecidable — the local
// newest-activity view is stale by definition, and a read-to-end cannot
// be told apart from a mark-unread at an uncached reply, which is a
// deliberate user signal that must not be guessed away. Callers persist
// the facts and skip the UI dispatch; the next getView reconcile settles
// it. String comparison is valid: Slack ts are fixed-width secs.micros.
func threadMarkReadState(lastRead, newestActivity string) (read, known bool) {
	if newestActivity == "" || lastRead > newestActivity {
		return false, false
	}
	return lastRead == newestActivity, true
}
