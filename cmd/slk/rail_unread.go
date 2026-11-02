package main

import (
	"github.com/gammons/slk/internal/cache"
)

// railUnreadWorkspaces returns the workspace IDs whose rail dot should
// be lit. It is the reader wireCallbacks installs through
// App.SetUnreadService; OtherUnreadCount (the title's "+N" and
// $SLK_OTHER_UNREAD) reads through the same reader, so the surfaces
// cannot disagree.
//
// A workspace is lit when its own sidebar would show something unread,
// and the sidebar has two such signals:
//
//   - channels: some channel in wctx.Channels is
//     ChannelItem.IsVisiblyUnread, the predicate the sidebar dot,
//     UnreadChannelCount and $SLK_UNREAD share, so a muted channel
//     never lights the rail.
//   - threads: the workspace's ThreadsBadge reports unread followed
//     threads, Slack's own has_unreads (see threadsBadge). A thread
//     reply never sets the channel's has_unread (OnMessage's
//     channelEligible), so this is the only way a thread reaches the
//     dot. Channel mute and wctx.Channels membership do not apply to
//     it, as they do not to the Threads badge.
//
// In production unread is db.UnreadChannels, teamIDs the configured
// workspace list and byID router.ByID; all are parameters so the
// predicate is pure.
//
// Two edge cases go opposite ways. byID returning nil (still
// connecting, or connect failed) lights on any unread channel row:
// nothing to check against, so MuteStore.Ready's conservative default
// applies and last session's dots survive boot. Its threads are not
// known until it connects, so they light nothing. A channel row whose
// channel is not in wctx.Channels never lights: the sidebar cannot
// show it, so there is no row to explain a dot and no keystroke to
// clear it.
//
// wctx.Channels is read on the UI goroutine without synchronization,
// the way wireCallbacks' Lookup callback already reads it (#208).
func railUnreadWorkspaces(unread []cache.UnreadChannel, teamIDs []string, byID func(teamID string) *WorkspaceContext) []string {
	var out []string
	lit := map[string]bool{}
	for _, u := range unread {
		if lit[u.WorkspaceID] {
			continue
		}
		if railRowLights(u, byID(u.WorkspaceID)) {
			lit[u.WorkspaceID] = true
			out = append(out, u.WorkspaceID)
		}
	}
	for _, teamID := range teamIDs {
		if lit[teamID] {
			continue
		}
		if wctx := byID(teamID); wctx != nil && wctx.ThreadsBadge.hasUnreads.Load() {
			lit[teamID] = true
			out = append(out, teamID)
		}
	}
	return out
}

// railRowLights reports whether one unread row lights its workspace's
// dot. The nil-wctx branch is the "unknown, so light it" case
// railUnreadWorkspaces documents; a channel absent from wctx.Channels
// is the "cannot be shown, so never light it" case.
func railRowLights(u cache.UnreadChannel, wctx *WorkspaceContext) bool {
	if wctx == nil {
		return true
	}
	for _, item := range wctx.Channels {
		if item.ID == u.ChannelID {
			return item.IsVisiblyUnread(u.State)
		}
	}
	return false
}
