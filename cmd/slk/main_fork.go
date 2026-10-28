package main

import (
	"fmt"
	"log"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackurl"
)

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
