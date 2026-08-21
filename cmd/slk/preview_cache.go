package main

import (
	"sync"
	"time"
)

const previewCacheTTL = 5 * time.Minute

// previewCache memoizes network-resolved link previews so reopening a
// picker doesn't refetch the same targets. Not-found results are
// cached too (as an empty entry), so a deleted or missing target isn't
// re-queried on every open. Safe for concurrent use: preview fetches
// run on parallel goroutines.
type previewCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]previewCacheEntry
}

type previewCacheEntry struct {
	userID, text string
	storedAt     time.Time
}

func newPreviewCache() *previewCache {
	return &previewCache{now: time.Now, entries: make(map[string]previewCacheEntry)}
}

func previewCacheKey(channelID, ts string) string { return channelID + "\x00" + ts }

// Get returns the entry for (channelID, ts); ok is false once an
// entry is older than previewCacheTTL.
func (c *previewCache) Get(channelID, ts string) (userID, text string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := previewCacheKey(channelID, ts)
	e, found := c.entries[key]
	if !found {
		return "", "", false
	}
	if c.now().Sub(e.storedAt) > previewCacheTTL {
		delete(c.entries, key)
		return "", "", false
	}
	return e.userID, e.text, true
}

func (c *previewCache) Put(channelID, ts, userID, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[previewCacheKey(channelID, ts)] = previewCacheEntry{userID: userID, text: text, storedAt: c.now()}
}
