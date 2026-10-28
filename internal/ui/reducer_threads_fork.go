package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// markFetchedThreadRead marks the open thread read up to the newest of
// the fetched replies when the user can see them: the pane's herdr tab
// is in view and the terminal has focus. Otherwise the mark is staged,
// as an arriving reply's is, and the refocus flush sends it. Not every
// fetch is one the user asked for: the refetch on reconnect after a
// laptop wake lands in every pane with a thread open, and Slack marks
// read only what is viewed.
func (a *App) markFetchedThreadRead(channelID, threadTS string, replies []messages.MessageItem) tea.Cmd {
	if !a.PaneViewed() || !a.terminalFocused {
		latestTS := threadTS
		if t := newestMarkableReplyTS(replies); t != "" {
			latestTS = t
		}
		a.recordThreadMark(channelID, threadTS, latestTS)
		return nil
	}
	cmd := a.markOpenThreadRead(channelID, threadTS, replies)
	a.markAgentThreadRead(a.activeTeamID, channelID, threadTS)
	return cmd
}

// markThreadReadLocally clears one thread's local unread state everywhere
// it is shown: the threads-list row, the sidebar's threads badge, and the
// agent-sidebar row. Every path that decides a thread has been read
// funnels here so a new one can't update two of the three and leave the
// herdr row claiming unread replies the user has already seen.
func (a *App) markThreadReadLocally(channelID, threadTS string) {
	if a.threadsView.MarkByThreadTSRead(channelID, threadTS) {
		a.sidebar.SetThreadsUnreadCount(a.threadsView.UnreadCount())
	}
	a.markAgentThreadRead(a.activeTeamID, channelID, threadTS)
}

// threadMarkedRemoteFork is the fork's half of the ThreadMarkedRemoteMsg
// arm; true means the message is fully handled.
//
// The agent thread row is served here for every workspace, from m.Read:
// it has no threads-list summary to derive read state from. A mark from
// a background workspace (see NewMessageMsg.TeamID) goes no further.
//
// A read mark never moves the open panel's "── new ──" landmark, whoever
// issued it: marks land while the user is watching the thread, and
// yanking the landmark mid-read would hide which replies were new. It
// takes the self-echo route — list state only — and consumes any
// self-mark record so a stale one can't shield a later unread mark.
// Unread marks fall through to upstream's applyThreadMarkEcho.
func (a *App) threadMarkedRemoteFork(m ThreadMarkedRemoteMsg) bool {
	teamID := m.TeamID
	if teamID == "" {
		teamID = a.activeTeamID
	}
	if m.Read {
		a.markAgentThreadRead(teamID, m.ChannelID, m.ThreadTS)
	} else {
		a.markAgentThreadUnread(teamID, m.ChannelID, m.ThreadTS, m.LastRead)
	}
	if teamID != a.activeTeamID {
		return true
	}
	if !m.Read {
		return false
	}
	a.selfThreadMarks.consume(selfMarkKey{channelID: m.ChannelID, threadTS: m.ThreadTS, ts: m.LastRead})
	a.applyThreadMarkListState(m.ChannelID, m.ThreadTS, m.LastRead)
	return true
}
