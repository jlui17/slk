package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/usernames"
)

const previewCacheTTL = 5 * time.Minute

// previewCache memoizes network-fetched link preview targets so
// reopening a picker doesn't refetch them. It holds what names the
// sender (user, bot id, username) and the text, not the sender's name: a
// name that was only an id at fetch time is resolved again each time
// the preview is served. The rest of the message is dropped. Not-found results are cached
// too (as an empty message), so a deleted or missing target isn't
// re-queried on every open. Safe for concurrent use: preview fetches
// run on parallel goroutines.
type previewCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]previewCacheEntry
}

type previewCacheEntry struct {
	message  slack.Message
	storedAt time.Time
}

func newPreviewCache() *previewCache {
	return &previewCache{now: time.Now, entries: make(map[string]previewCacheEntry)}
}

func previewCacheKey(channelID, ts string) string { return channelID + "\x00" + ts }

// Get returns the entry for (channelID, ts); ok is false once an
// entry is older than previewCacheTTL.
func (c *previewCache) Get(channelID, ts string) (m slack.Message, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := previewCacheKey(channelID, ts)
	e, found := c.entries[key]
	if !found {
		return slack.Message{}, false
	}
	if c.now().Sub(e.storedAt) > previewCacheTTL {
		delete(c.entries, key)
		return slack.Message{}, false
	}
	return e.message, true
}

func (c *previewCache) Put(channelID, ts string, m slack.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := slack.Message{Msg: slack.Msg{User: m.User, BotID: m.BotID, Username: m.Username, Text: m.Text}}
	c.entries[previewCacheKey(channelID, ts)] = previewCacheEntry{message: kept, storedAt: c.now()}
}

// cachedMessagePreview is the Preview seam's cache tier: the target
// message's sender and raw text when the cache holds it. A tombstone
// (soft-deleted row; GetMessage is the one cache read without an
// is_deleted filter) counts as found with no text, so the network
// tier isn't asked to resurrect a message we know is gone.
//
// The sender is read off the row's raw message, as previewSender wants
// it: a bot post carries its own username there, and its user_id column
// holds only the bot's id. A row without a raw message has the column.
func cachedMessagePreview(db *cache.DB, names *usernames.Store, router *workspaceRouter, channelID, ts string) (sender, text string, found bool) {
	m, err := db.GetMessage(channelID, ts)
	if err != nil {
		return "", "", false
	}
	if m.IsDeleted {
		return "", "", true
	}
	var raw slack.Message
	if json.Unmarshal([]byte(m.RawJSON), &raw) != nil || raw.User == "" && raw.BotID == "" {
		raw.User = m.UserID
	}
	return previewSender(raw, names, db, router), m.Text, true
}

// fetchedMessagePreview is the Preview seam's network tier, memoized in
// c.
func fetchedMessagePreview(c *previewCache, fetch func() (*slack.Message, error), names *usernames.Store, db *cache.DB, router *workspaceRouter, channelID, ts string) (sender, text string, err error) {
	m, ok := c.Get(channelID, ts)
	if !ok {
		fetched, err := fetch()
		if err != nil {
			return "", "", err
		}
		if fetched != nil {
			m = *fetched
		}
		c.Put(channelID, ts, m)
	}
	return previewSender(m, names, db, router), m.Text, nil
}

// previewSender is the name the message pane's header shows for m, by
// the pane's own rule (messageAuthor): a bot post's username first, one
// bot can post under several, then the name known for the user or bot
// id, then the id. "" when the message names no one.
func previewSender(m slack.Message, names *usernames.Store, db *cache.DB, router *workspaceRouter) string {
	_, name := messageAuthor(m, newUserNameFill(names), db, router)
	return name
}
