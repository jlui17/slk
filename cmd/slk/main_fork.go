package main

import (
	"fmt"
	"log"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackurl"
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

// resolveStartupWorkspace picks the workspace that claims the initial
// active slot when a command-line permalink or a pane-state restore asks
// for one, starting from defaultTeamID (default_workspace, already
// checked against tokens), and queues the startup navigation through
// setStartupLink. It returns the team to claim, the team the pane state
// forced (empty unless the restore picked it), and the pane state still
// to restore (nil once the link or a missing token overrides it).
func resolveStartupWorkspace(tokens []slackclient.Token, startupLink *slackurl.Permalink, paneRestore *cache.PaneState, defaultTeamID string, setStartupLink func(channelID, messageTS, threadTS string)) (teamID, paneForcedTeam string, restore *cache.PaneState, err error) {
	// A command-line permalink overrides default_workspace: the link's
	// workspace must claim the initial active slot for the startup
	// navigation queued below to run. It also overrides the pane-state
	// restore for this launch; the link becomes the recorded state the
	// moment it opens.
	if startupLink != nil {
		paneRestore = nil
		linkTeamID := ""
		for _, t := range tokens {
			if t.Domain == startupLink.Subdomain {
				linkTeamID = t.TeamID
				break
			}
		}
		if linkTeamID == "" {
			return "", "", nil, fmt.Errorf("link is for %s.slack.com, which is not a configured workspace (see slk --list-workspaces)", startupLink.Subdomain)
		}
		defaultTeamID = linkTeamID
		setStartupLink(string(startupLink.ChannelID), string(startupLink.MessageTS), string(startupLink.ThreadTS))
	}

	// Pane-state restore claims the initial active slot the same way a
	// permalink does. A recorded thread rides the permalink startup-nav
	// path (channel select, thread panel, off-buffer backfill, missing
	// channel degrading to a toast); a channel-only state flows through
	// WorkspaceReadyMsg.LastChannelID via restoredChannelFor.
	//
	// paneForcedTeam remembers that the team came from auto-recorded
	// state rather than the user's default_workspace config: if that
	// workspace then fails to connect, still-pending workspaces fall back
	// to first-to-connect-wins (see the connect-failure path in run)
	// instead of leaving slk with no active workspace at all.
	if paneRestore != nil {
		found := false
		for _, t := range tokens {
			if t.TeamID == paneRestore.WorkspaceID {
				found = true
				break
			}
		}
		if !found {
			log.Printf("Warning: pane state names workspace %q but no token is configured for it; ignoring", paneRestore.WorkspaceID)
			paneRestore = nil
		} else {
			defaultTeamID = paneRestore.WorkspaceID
			paneForcedTeam = paneRestore.WorkspaceID
			if paneRestore.ThreadTS != "" {
				setStartupLink(paneRestore.ChannelID, paneRestore.ThreadTS, paneRestore.ThreadTS)
			}
		}
	}
	return defaultTeamID, paneForcedTeam, paneRestore, nil
}
