package main

import (
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/usernames"
)

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

// authorName is the name a message shows for its author: a bot
// message's own username when it carries one, as Slack shows it (one
// bot can post under a different username per message, through
// chat.postMessage's username override), else stored, the author's
// name from the store or cache. A message with a user is not a bot
// message, whatever username it carries.
func authorName(userID, username, stored string) string {
	if userID == "" && username != "" {
		return username
	}
	return stored
}
